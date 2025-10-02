package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/nats-io/nats.go"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
)

type ServerConfig struct {
	Addr           string
	NATSUrl        string
	MaxConnections int
	BufferSize     int
	WorkerCount    int
}

type Server struct {
	config   ServerConfig
	logger   *log.Logger
	listener net.Listener
	natsConn *nats.Conn

	// Connection management
	connections     *ConnectionPool
	clients         sync.Map // map[*Client]bool
	clientCount     int64
	connectionsSem  chan struct{} // Semaphore for max connections

	// Performance optimization
	bufferPool *BufferPool
	workerPool *WorkerPool

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Stats
	stats *Stats
}

type Stats struct {
	TotalConnections   int64
	CurrentConnections int64
	MessagesSent       int64
	MessagesReceived   int64
	BytesSent          int64
	BytesReceived      int64
	StartTime          time.Time
	mu                 sync.RWMutex
	CPUPercent         float64
	MemoryMB           float64
}

func NewServer(config ServerConfig, logger *log.Logger) (*Server, error) {
	ctx, cancel := context.WithCancel(context.Background())

	s := &Server{
		config:         config,
		logger:         logger,
		ctx:            ctx,
		cancel:         cancel,
		connections:    NewConnectionPool(config.MaxConnections),
		connectionsSem: make(chan struct{}, config.MaxConnections),
		bufferPool:     NewBufferPool(config.BufferSize),
		workerPool:     NewWorkerPool(config.WorkerCount),
		stats: &Stats{
			StartTime: time.Now(),
		},
	}

	if config.NATSUrl != "" {
		nc, err := nats.Connect(config.NATSUrl, nats.MaxReconnects(5), nats.ReconnectWait(2*time.Second))
		if err != nil {
			return nil, fmt.Errorf("failed to connect to nats: %w", err)
		}
		s.natsConn = nc
		logger.Printf("Connected to NATS at %s", config.NATSUrl)
	}

	return s, nil
}

func (s *Server) Start() error {
	// Simple TCP listener without custom socket control
	// Custom socket options were causing bind issues in containers
	listener, err := net.Listen("tcp", s.config.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	s.listener = listener

	s.logger.Printf("Server listening on %s", s.config.Addr)

	s.workerPool.Start(s.ctx)

	if s.natsConn != nil {
		msgCount := int64(0)
		// Subscribe to all token updates using wildcard (matches Node.js server)
		_, err := s.natsConn.Subscribe("odin.token.>", func(msg *nats.Msg) {
			count := atomic.AddInt64(&msgCount, 1)
			if count%100 == 0 {
				s.logger.Printf("Received %d messages from NATS", count)
			}
			// Submit the broadcast job to the worker pool to avoid blocking the NATS callback
			s.workerPool.Submit(func() {
				s.broadcast(msg.Data)
			})
		})
		if err != nil {
			return fmt.Errorf("failed to subscribe to nats: %w", err)
		}
		s.logger.Printf("Subscribed to NATS topic: odin.token.> (all token updates)")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/stats", s.handleStats)

	server := &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			s.logger.Printf("Server error: %v", err)
		}
	}()

	// Start metrics collection
	s.wg.Add(1)
	go s.collectMetrics()

	return nil
}

func (s *Server) collectMetrics() {
	defer s.wg.Done()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Get current process
	proc, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		s.logger.Printf("Failed to get process: %v", err)
		proc = nil
	}

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// Get CPU percentage
			cpuPercent, err := cpu.Percent(0, false)
			if err == nil && len(cpuPercent) > 0 {
				s.stats.mu.Lock()
				s.stats.CPUPercent = cpuPercent[0]
				s.stats.mu.Unlock()
			}

			// Get memory usage
			if proc != nil {
				memInfo, err := proc.MemoryInfo()
				if err == nil {
					memMB := float64(memInfo.RSS) / 1024 / 1024
					s.stats.mu.Lock()
					s.stats.MemoryMB = memMB
					s.stats.mu.Unlock()
				}
			} else {
				// Fallback to system memory
				vmem, err := mem.VirtualMemory()
				if err == nil {
					memMB := float64(vmem.Used) / 1024 / 1024
					s.stats.mu.Lock()
					s.stats.MemoryMB = memMB
					s.stats.mu.Unlock()
				}
			}
		}
	}
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Try to acquire connection slot (non-blocking with timeout)
	select {
	case s.connectionsSem <- struct{}{}:
		// Slot acquired
	case <-time.After(5 * time.Second):
		// Server at capacity, reject with 503
		http.Error(w, "Server at capacity", http.StatusServiceUnavailable)
		return
	}

	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		<-s.connectionsSem // Release slot
		s.logger.Printf("Failed to upgrade connection: %v", err)
		return
	}

	client := s.connections.Get()
	client.conn = conn
	client.server = s
	client.id = atomic.AddInt64(&s.clientCount, 1)

	s.clients.Store(client, true)
	atomic.AddInt64(&s.stats.TotalConnections, 1)
	atomic.AddInt64(&s.stats.CurrentConnections, 1)

	go s.writePump(client)
	go s.readPump(client)
}

func (s *Server) readPump(c *Client) {
	defer func() {
		c.conn.Close()
		s.clients.Delete(c)
		atomic.AddInt64(&s.stats.CurrentConnections, -1)
		s.connections.Put(c)
		<-s.connectionsSem // Release connection slot
	}()

	c.conn.SetReadDeadline(time.Now().Add(pongWait))

	for {
		msg, op, err := wsutil.ReadClientData(c.conn)
		if err != nil {
			break
		}

		c.conn.SetReadDeadline(time.Now().Add(pongWait))

		atomic.AddInt64(&s.stats.MessagesReceived, 1)
		atomic.AddInt64(&s.stats.BytesReceived, int64(len(msg)))

		if op == ws.OpText {
			// Messages from clients are not processed in this version
		} else if op == ws.OpPing {
			// The gobwas library handles pongs automatically by default
		} else if op == ws.OpClose {
			break
		}
	}
}

func (s *Server) writePump(c *Client) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		if c.conn != nil {
			c.conn.Close()
		}
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				// The server closed the channel.
				if c.conn != nil {
					wsutil.WriteServerMessage(c.conn, ws.OpClose, []byte{})
				}
				return
			}

			if c.conn == nil {
				return
			}

			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			err := wsutil.WriteServerMessage(c.conn, ws.OpText, message)
			if err != nil {
				return
			}
			atomic.AddInt64(&s.stats.MessagesSent, 1)
			atomic.AddInt64(&s.stats.BytesSent, int64(len(message)))

		case <-ticker.C:
			if c.conn == nil {
				return
			}
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := wsutil.WriteServerMessage(c.conn, ws.OpPing, nil); err != nil {
				return
			}
		}
	}
}

func (s *Server) broadcast(message []byte) {
	s.clients.Range(func(key, value interface{}) bool {
		if client, ok := key.(*Client); ok {
			select {
			case client.send <- message:
			default:
				// Send buffer is full. The client is too slow.
				// We can disconnect the client or just drop the message.
				// For this high-performance case, we'll drop and let the read pump handle eventual timeouts.
			}
		}
		return true
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Handle preflight requests
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	health := map[string]interface{}{
		"status":      "healthy",
		"connections": atomic.LoadInt64(&s.stats.CurrentConnections),
		"uptime":      time.Since(s.stats.StartTime).Seconds(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Handle preflight requests
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	s.stats.mu.RLock()
	cpuPercent := s.stats.CPUPercent
	memoryMB := s.stats.MemoryMB
	s.stats.mu.RUnlock()

	stats := map[string]interface{}{
		"totalConnections":   atomic.LoadInt64(&s.stats.TotalConnections),
		"currentConnections": atomic.LoadInt64(&s.stats.CurrentConnections),
		"messagesSent":       atomic.LoadInt64(&s.stats.MessagesSent),
		"messagesReceived":   atomic.LoadInt64(&s.stats.MessagesReceived),
		"bytesSent":          atomic.LoadInt64(&s.stats.BytesSent),
		"bytesReceived":      atomic.LoadInt64(&s.stats.BytesReceived),
		"uptime":             time.Since(s.stats.StartTime).Seconds(),
		"cpuPercent":         cpuPercent,
		"memoryMB":           memoryMB,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) Shutdown() error {
	s.cancel()

	if s.listener != nil {
		s.listener.Close()
	}

	if s.natsConn != nil {
		s.natsConn.Close()
	}

	s.clients.Range(func(key, value interface{}) bool {
		if client, ok := key.(*Client); ok {
			close(client.send)
		}
		return true
	})

	s.workerPool.Stop()
	s.wg.Wait()

	return nil
}

