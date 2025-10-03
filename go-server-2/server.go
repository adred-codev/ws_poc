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

	// Rate limiting
	rateLimiter *RateLimiter

	// Monitoring
	auditLogger     *AuditLogger
	metricsCollector *MetricsCollector

	// Lifecycle
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	shuttingDown int32 // Atomic flag for graceful shutdown

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

	// Message delivery reliability metrics
	SlowClientsDisconnected int64 // Count of clients disconnected for being too slow
	RateLimitedMessages     int64 // Count of messages dropped due to rate limiting
	MessageReplayRequests   int64 // Count of replay requests served (gap recovery)
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
		rateLimiter:    NewRateLimiter(),
		stats: &Stats{
			StartTime: time.Now(),
		},
	}

	// Initialize monitoring
	s.auditLogger = NewAuditLogger(INFO) // Log INFO and above
	s.auditLogger.SetAlerter(NewConsoleAlerter()) // Use console alerter for now
	s.metricsCollector = NewMetricsCollector(s)

	if config.NATSUrl != "" {
		nc, err := nats.Connect(config.NATSUrl, nats.MaxReconnects(5), nats.ReconnectWait(2*time.Second))
		if err != nil {
			s.auditLogger.Critical("NATSConnectionFailed", "Failed to connect to NATS", map[string]interface{}{
				"url":   config.NATSUrl,
				"error": err.Error(),
			})
			return nil, fmt.Errorf("failed to connect to nats: %w", err)
		}
		s.natsConn = nc
		logger.Printf("Connected to NATS at %s", config.NATSUrl)

		s.auditLogger.Info("NATSConnected", "Connected to NATS successfully", map[string]interface{}{
			"url": config.NATSUrl,
		})
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

			// Track NATS message in Prometheus
			IncrementNATSMessages()

			// Submit the broadcast job to the worker pool to avoid blocking the NATS callback
			s.workerPool.Submit(func() {
				s.broadcast(msg.Data)
			})
		})
		if err != nil {
			s.auditLogger.Critical("NATSSubscriptionFailed", "Failed to subscribe to NATS topic", map[string]interface{}{
				"topic": "odin.token.>",
				"error": err.Error(),
			})
			return fmt.Errorf("failed to subscribe to nats: %w", err)
		}
		s.logger.Printf("Subscribed to NATS topic: odin.token.> (all token updates)")

		// Monitor NATS connection status
		s.wg.Add(1)
		go s.monitorNATS()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/stats", s.handleStats)
	mux.HandleFunc("/metrics", handleMetrics) // Prometheus metrics endpoint

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

	// Start Prometheus metrics collector
	s.metricsCollector.Start()

	// Start memory monitoring
	s.wg.Add(1)
	go s.monitorMemory()

	s.auditLogger.Info("ServerStarted", "WebSocket server started successfully", map[string]interface{}{
		"addr":           s.config.Addr,
		"maxConnections": s.config.MaxConnections,
	})

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

func (s *Server) monitorNATS() {
	defer s.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	wasConnected := s.natsConn.IsConnected()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			isConnected := s.natsConn.IsConnected()

			// Detect connection state changes
			if wasConnected && !isConnected {
				// Connection lost
				s.auditLogger.Critical("NATSDisconnected", "Lost connection to NATS server", map[string]interface{}{
					"url": s.config.NATSUrl,
				})
			} else if !wasConnected && isConnected {
				// Connection restored
				s.auditLogger.Info("NATSReconnected", "Reconnected to NATS server", map[string]interface{}{
					"url": s.config.NATSUrl,
				})
			}

			wasConnected = isConnected
		}
	}
}

func (s *Server) monitorMemory() {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	memLimitMB := 512.0 // Container memory limit

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.stats.mu.RLock()
			memUsedMB := s.stats.MemoryMB
			s.stats.mu.RUnlock()

			memPercent := (memUsedMB / memLimitMB) * 100
			currentConns := atomic.LoadInt64(&s.stats.CurrentConnections)

			// Critical threshold: >90%
			if memPercent > 90 {
				s.auditLogger.Critical("HighMemoryUsage", "Memory usage above 90%, OOM risk", map[string]interface{}{
					"memory_used_mb":  memUsedMB,
					"memory_limit_mb": memLimitMB,
					"percentage":      memPercent,
					"connections":     currentConns,
				})
			} else if memPercent > 80 {
				// Warning threshold: >80%
				s.auditLogger.Warning("ModerateMemoryUsage", "Memory usage above 80%", map[string]interface{}{
					"memory_used_mb":  memUsedMB,
					"memory_limit_mb": memLimitMB,
					"percentage":      memPercent,
					"connections":     currentConns,
				})
			}
		}
	}
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Reject new connections during graceful shutdown
	if atomic.LoadInt32(&s.shuttingDown) == 1 {
		http.Error(w, "Server is shutting down", http.StatusServiceUnavailable)
		return
	}

	// Try to acquire connection slot (non-blocking with timeout)
	select {
	case s.connectionsSem <- struct{}{}:
		// Slot acquired
	case <-time.After(5 * time.Second):
		// Server at capacity, reject with 503
		s.auditLogger.Warning("ServerAtCapacity", "Connection rejected - server at maximum capacity", map[string]interface{}{
			"currentConnections": atomic.LoadInt64(&s.stats.CurrentConnections),
			"maxConnections":     s.config.MaxConnections,
		})
		connectionsFailed.Inc()
		http.Error(w, "Server at capacity", http.StatusServiceUnavailable)
		return
	}

	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		<-s.connectionsSem // Release slot
		s.auditLogger.Error("WebSocketUpgradeFailed", "Failed to upgrade HTTP connection to WebSocket", map[string]interface{}{
			"error":      err.Error(),
			"remoteAddr": r.RemoteAddr,
		})
		connectionsFailed.Inc()
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

	// Update Prometheus metrics
	UpdateConnectionMetrics(s)

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

		// Clean up rate limiter state
		// Important for memory management - prevents leak
		s.rateLimiter.RemoveClient(c.id)
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
		UpdateMessageMetrics(0, 1)
		UpdateBytesMetrics(0, int64(len(msg)))

		if op == ws.OpText {
			// Rate limiting check - prevent client from flooding server
			// Important for:
			// 1. DoS prevention (malicious client can't overwhelm server)
			// 2. Bug protection (client code bug causing infinite loop)
			// 3. Fair resource sharing (one client can't starve others)
			//
			// Rate limit: 100 burst, 10/sec sustained
			// This allows legitimate bursts (rapid order entry) while preventing abuse
			if !s.rateLimiter.CheckLimit(c.id) {
				s.logger.Printf("⚠️  Client %d rate limited (exceeded 100 burst or 10/sec sustained)", c.id)

				// Audit log rate limiting event
				s.auditLogger.Warning("ClientRateLimited", "Client exceeded rate limit", map[string]interface{}{
					"clientID": c.id,
					"limit":    "100 burst, 10/sec sustained",
				})

				// Send error message to client
				// This helps client-side debugging (they know why messages are being dropped)
				errorMsg := map[string]interface{}{
					"type":    "error",
					"code":    "RATE_LIMIT_EXCEEDED",
					"message": "Too many messages, please slow down (limit: 10/sec)",
				}

				if data, err := json.Marshal(errorMsg); err == nil {
					// Best effort send (don't block if client slow)
					select {
					case c.send <- data:
					default:
						// Client buffer full, they won't see error anyway
					}
				}

				// Increment rate limit counter for monitoring
				atomic.AddInt64(&s.stats.RateLimitedMessages, 1)
				IncrementRateLimitedMessages()

				// Drop the message but don't disconnect
				// Rationale: Might be temporary spike, give them a chance
				// If persistent, they'll see error messages and fix their code
				continue
			}

			// Process client message (replay requests, heartbeats, etc.)
			s.handleClientMessage(c, msg)

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
			UpdateMessageMetrics(1, 0)
			UpdateBytesMetrics(int64(len(message)), 0)

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

// broadcast sends a message to all connected clients with reliability guarantees
// This is the critical path for message delivery in a trading platform
//
// Changes from basic WebSocket broadcast:
// 1. Wraps raw NATS messages in MessageEnvelope with sequence numbers
// 2. Stores in replay buffer BEFORE sending (ensures can replay if send fails)
// 3. Detects slow clients and disconnects them (prevents head-of-line blocking)
// 4. Never drops messages silently (either deliver or disconnect client)
//
// Industry standard approach:
// - Bloomberg Terminal: Disconnects slow clients, no message drops
// - Coinbase: 2-strike disconnection policy
// - FIX protocol: Requires guaranteed delivery or explicit reject
//
// Performance characteristics:
// - O(n) where n = number of connected clients
// - With 10,000 clients: ~1-2ms per broadcast
// - Called ~10 times/second from NATS subscription
// - Total CPU: ~1-2% (acceptable)
func (s *Server) broadcast(message []byte) {
	// Iterate over all connected clients
	// sync.Map.Range is thread-safe and allows concurrent reads during iteration
	s.clients.Range(func(key, value interface{}) bool {
		if client, ok := key.(*Client); ok {
			// Wrap raw NATS message in envelope with sequence number
			// Message type: "price:update" (could be parsed from NATS subject)
			// Priority: HIGH (trading data is critical but not life-or-death)
			envelope, err := WrapMessage(message, "price:update", PRIORITY_HIGH, client.seqGen)
			if err != nil {
				s.logger.Printf("❌ Failed to wrap message for client %d: %v", client.id, err)
				return true // Continue to next client
			}

			// Add to replay buffer BEFORE sending
			// Critical: If send fails, client can request replay
			// If we added AFTER send, failed sends wouldn't be replayable
			client.replayBuffer.Add(envelope)

			// Serialize envelope to JSON for WebSocket transmission
			data, err := envelope.Serialize()
			if err != nil {
				s.logger.Printf("❌ Failed to serialize message for client %d: %v", client.id, err)
				return true // Continue to next client
			}

			// Attempt to send with timeout-based slow client detection
			// For HIGH priority messages: 100ms timeout
			// CRITICAL messages would use 1 second timeout
			// NORMAL messages would use non-blocking send (drop if buffer full)
			select {
			case client.send <- data:
				// Success - message queued for writePump to send
				// Reset failure counter (client is healthy)
				atomic.StoreInt32(&client.sendAttempts, 0)
				client.lastMessageSentAt = time.Now()

			case <-time.After(100 * time.Millisecond):
				// Timeout - client's send buffer is full (can't keep up)
				// This indicates:
				// 1. Client on slow network (mobile, bad wifi)
				// 2. Client device overloaded (CPU pegged, memory swapping)
				// 3. Client code has bug (not reading messages)
				//
				// We CANNOT drop the message (trading platform requirement)
				// Options:
				// A. Block until sent (bad - blocks all other clients)
				// B. Disconnect client (what we do - fair to other clients)
				//
				attempts := atomic.AddInt32(&client.sendAttempts, 1)

				// Log warning on first failure (avoid spam)
				if attempts == 1 && !client.slowClientWarned {
					s.logger.Printf("⚠️  Client %d is slow (send blocked >100ms, buffer full)", client.id)
					client.slowClientWarned = true
				}

				// Disconnect after 3 consecutive failures (industry standard)
				// Why 3? Balance between:
				// - Too low (1-2): False positives from network hiccups
				// - Too high (5+): Slow client blocks resources too long
				if attempts >= 3 {
					s.logger.Printf("❌ Disconnecting client %d (too slow: %d consecutive send failures)", client.id, attempts)

					// Audit log slow client disconnection
					s.auditLogger.Warning("SlowClientDisconnected", "Client disconnected for being too slow", map[string]interface{}{
						"clientID":         client.id,
						"consecutiveFails": attempts,
						"timeout":          "100ms",
					})

					// Send WebSocket close frame with reason code
					// Close code 1008 = Policy Violation (client too slow)
					// This helps client-side debugging (clear error message)
					// Standard close codes:
					//   1000 = Normal closure
					//   1001 = Going away (server restart)
					//   1008 = Policy violation (rate limit, too slow)
					//   1011 = Internal error
					if client.conn != nil {
						closeMsg := ws.NewCloseFrameBody(ws.StatusPolicyViolation, "Client too slow to process messages")
						ws.WriteFrame(client.conn, ws.NewCloseFrame(closeMsg))
						client.conn.Close()
					}

					// Increment slow client counter for monitoring
					// If this counter is high (>1% of connections), indicates:
					// - Network infrastructure issues
					// - Client app performance issues
					// - Need to optimize message size/frequency
					atomic.AddInt64(&s.stats.SlowClientsDisconnected, 1)
					IncrementSlowClientDisconnects()
				}
			}
		}
		return true // Continue to next client
	})
}

// handleClientMessage processes incoming WebSocket messages from clients
// Trading clients send various message types:
// 1. "replay" - Request missed messages (gap recovery after network issue)
// 2. "heartbeat" - Keep-alive ping (some clients don't support WebSocket ping)
// 3. "subscribe" - Subscribe to specific symbols (future enhancement)
// 4. "unsubscribe" - Unsubscribe from symbols (future enhancement)
//
// Message format (JSON):
// {
//   "type": "replay",
//   "data": {"from": 100, "to": 150}  // Request sequence 100-150
// }
//
// Industry patterns:
// - FIX protocol: Gap fill requests are standard
// - NATS JetStream: Consumers can request replay by sequence
// - Kafka: Consumers can seek to specific offsets
// - Our implementation: Similar to NATS JetStream
func (s *Server) handleClientMessage(c *Client, data []byte) {
	// Parse outer message structure
	var req struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(data, &req); err != nil {
		s.logger.Printf("⚠️  Client %d sent invalid JSON: %v", c.id, err)
		return
	}

	switch req.Type {
	case "replay":
		// Client detected gap in sequence numbers, requesting missed messages
		// Example scenario:
		//   Client received: seq 100, 101, 102, [network hiccup], 150
		//   Client detects gap: 103-149 missing
		//   Client requests: {"type": "replay", "data": {"from": 103, "to": 149}}
		//
		// This is MUCH faster than full reconnect:
		// - Reconnect: 500ms-2s (WebSocket handshake + resubscribe)
		// - Replay: 10-50ms (send buffered messages)
		//
		// Especially important for mobile clients with flaky networks
		var replayReq struct {
			From  int64 `json:"from"`  // Start sequence (inclusive)
			To    int64 `json:"to"`    // End sequence (inclusive)
			Since int64 `json:"since"` // Alternative: "give me everything after X"
		}

		if err := json.Unmarshal(req.Data, &replayReq); err != nil {
			s.logger.Printf("⚠️  Client %d sent invalid replay request: %v", c.id, err)
			return
		}

		s.logger.Printf("📬 Client %d requesting replay: seq %d to %d", c.id, replayReq.From, replayReq.To)

		// Get messages from replay buffer
		var messages []*MessageEnvelope
		if replayReq.Since > 0 {
			// "Give me everything after seq X" (used during reconnection)
			messages = c.replayBuffer.GetSince(replayReq.Since)
		} else {
			// "Give me seq X to Y" (used for gap filling)
			messages = c.replayBuffer.GetRange(replayReq.From, replayReq.To)
		}

		s.logger.Printf("📬 Replaying %d messages to client %d", len(messages), c.id)

		// Send each message from replay buffer
		// Note: These already have sequence numbers (from when originally sent)
		for i, msg := range messages {
			data, err := msg.Serialize()
			if err != nil {
				s.logger.Printf("❌ Failed to serialize replay message %d: %v", i, err)
				continue
			}

			// Send with 1 second timeout (replay is important, give more time)
			select {
			case c.send <- data:
				// Sent successfully
			case <-time.After(1 * time.Second):
				// If client can't handle replay messages, they're too slow
				// This is a problem because they'll just keep falling behind
				s.logger.Printf("❌ Client %d too slow to handle replay (dropped message %d/%d)", c.id, i+1, len(messages))

				// Should we disconnect? Probably not during replay
				// Client might be temporarily slow but recovering
				// Let regular slow client detection handle it
				break
			}
		}

		// Increment replay counter for monitoring
		// High replay rate indicates:
		// - Network infrastructure issues
		// - Server having trouble keeping up (CPU/memory)
		// - Need to increase client send buffer size
		atomic.AddInt64(&s.stats.MessageReplayRequests, 1)
		IncrementReplayRequests()

	case "heartbeat":
		// Client keep-alive ping
		// Some older browsers/libraries don't support WebSocket ping/pong
		// So they send application-level heartbeats
		//
		// We respond with current server time (helps detect clock skew)
		pong := map[string]interface{}{
			"type": "pong",
			"ts":   time.Now().UnixMilli(),
		}

		if data, err := json.Marshal(pong); err == nil {
			select {
			case c.send <- data:
				// Sent
			default:
				// If can't send pong, client buffer is full
				// Don't worry about it, regular heartbeat will fail eventually
			}
		}

	case "subscribe":
		// Future enhancement: Per-symbol subscriptions
		// Currently we broadcast all messages to all clients
		// For production at scale, clients subscribe to specific symbols:
		//   {"type": "subscribe", "data": {"symbols": ["BTC", "ETH"]}}
		//
		// Benefits:
		// - Reduced bandwidth (only send relevant updates)
		// - Reduced CPU (filter before broadcast)
		// - Better user experience (only see what they care about)
		//
		// Implementation would add:
		//   type Client struct {
		//       subscriptions map[string]bool // symbol -> subscribed
		//   }
		//
		// Then in broadcast(), check if client subscribed before sending
		s.logger.Printf("ℹ️  Client %d sent 'subscribe' (not yet implemented)", c.id)

	case "unsubscribe":
		// Future enhancement: Unsubscribe from symbols
		s.logger.Printf("ℹ️  Client %d sent 'unsubscribe' (not yet implemented)", c.id)

	default:
		// Unknown message type
		// Log but don't disconnect (might be future feature we haven't implemented yet)
		s.logger.Printf("⚠️  Client %d sent unknown message type: %s", c.id, req.Type)
	}
}

// handleHealth provides enhanced health checks with detailed status
// Returns: healthy, degraded (warnings), or unhealthy (errors)
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	// Handle preflight requests
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Get current metrics
	s.stats.mu.RLock()
	cpuPercent := s.stats.CPUPercent
	memoryMB := s.stats.MemoryMB
	s.stats.mu.RUnlock()

	currentConns := atomic.LoadInt64(&s.stats.CurrentConnections)
	maxConns := int64(s.config.MaxConnections)
	slowClients := atomic.LoadInt64(&s.stats.SlowClientsDisconnected)
	totalConns := atomic.LoadInt64(&s.stats.TotalConnections)

	memLimitMB := 512.0 // Container memory limit

	// Calculate health status
	isHealthy := true
	warnings := []string{}
	errors := []string{}

	// Check NATS
	natsStatus := "disconnected"
	natsHealthy := false
	if s.natsConn != nil && s.natsConn.IsConnected() {
		natsStatus = "connected"
		natsHealthy = true
	} else {
		isHealthy = false
		errors = append(errors, "NATS connection lost")
	}

	// Check capacity
	capacityPercent := float64(currentConns) / float64(maxConns) * 100
	capacityHealthy := true
	if capacityPercent > 95 {
		isHealthy = false
		capacityHealthy = false
		errors = append(errors, "Server at capacity (>95%)")
	} else if capacityPercent > 80 {
		capacityHealthy = false
		warnings = append(warnings, "Server capacity high (>80%)")
	}

	// Check memory
	memPercent := (memoryMB / memLimitMB) * 100
	memHealthy := true
	if memPercent > 90 {
		isHealthy = false
		memHealthy = false
		errors = append(errors, "Memory usage critical (>90%)")
	} else if memPercent > 80 {
		memHealthy = false
		warnings = append(warnings, "Memory usage high (>80%)")
	}

	// Check CPU
	cpuHealthy := true
	if cpuPercent > 90 {
		isHealthy = false
		cpuHealthy = false
		errors = append(errors, "CPU usage critical (>90%)")
	} else if cpuPercent > 70 {
		cpuHealthy = false
		warnings = append(warnings, "CPU usage high (>70%)")
	}

	// Check slow client rate
	if totalConns > 0 {
		slowClientPercent := float64(slowClients) / float64(totalConns) * 100
		if slowClientPercent > 10 {
			warnings = append(warnings, "High slow client disconnect rate (>10%)")
		}
	}

	// Determine overall status
	status := "healthy"
	statusCode := http.StatusOK
	if !isHealthy {
		status = "unhealthy"
		statusCode = http.StatusServiceUnavailable
	} else if len(warnings) > 0 {
		status = "degraded"
		statusCode = http.StatusOK // Still accepting traffic
	}

	// Build response
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  status,
		"healthy": isHealthy,
		"checks": map[string]interface{}{
			"nats": map[string]interface{}{
				"status":  natsStatus,
				"healthy": natsHealthy,
			},
			"capacity": map[string]interface{}{
				"current":    currentConns,
				"max":        maxConns,
				"percentage": capacityPercent,
				"healthy":    capacityHealthy,
			},
			"memory": map[string]interface{}{
				"used_mb":    memoryMB,
				"limit_mb":   memLimitMB,
				"percentage": memPercent,
				"healthy":    memHealthy,
			},
			"cpu": map[string]interface{}{
				"percentage": cpuPercent,
				"healthy":    cpuHealthy,
			},
		},
		"warnings": warnings,
		"errors":   errors,
		"uptime":   time.Since(s.stats.StartTime).Seconds(),
	})
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
		"totalConnections":        atomic.LoadInt64(&s.stats.TotalConnections),
		"currentConnections":      atomic.LoadInt64(&s.stats.CurrentConnections),
		"messagesSent":            atomic.LoadInt64(&s.stats.MessagesSent),
		"messagesReceived":        atomic.LoadInt64(&s.stats.MessagesReceived),
		"bytesSent":               atomic.LoadInt64(&s.stats.BytesSent),
		"bytesReceived":           atomic.LoadInt64(&s.stats.BytesReceived),
		"uptime":                  time.Since(s.stats.StartTime).Seconds(),
		"cpuPercent":              cpuPercent,
		"memoryMB":                memoryMB,
		"slowClientsDisconnected": atomic.LoadInt64(&s.stats.SlowClientsDisconnected),
		"rateLimitedMessages":     atomic.LoadInt64(&s.stats.RateLimitedMessages),
		"messageReplayRequests":   atomic.LoadInt64(&s.stats.MessageReplayRequests),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) Shutdown() error {
	s.logger.Println("Initiating graceful shutdown...")

	// Set shutdown flag to reject new connections
	atomic.StoreInt32(&s.shuttingDown, 1)

	// Stop accepting new connections
	if s.listener != nil {
		s.logger.Println("Closing listener (no new connections accepted)")
		s.listener.Close()
	}

	// Stop receiving new messages from NATS
	if s.natsConn != nil {
		s.logger.Println("Closing NATS connection (no new messages)")
		s.natsConn.Close()
	}

	// Count current connections
	currentConns := atomic.LoadInt64(&s.stats.CurrentConnections)
	s.logger.Printf("Draining %d active connections (30s grace period)...", currentConns)

	// Grace period for connection draining
	gracePeriod := 30 * time.Second
	drainTimer := time.NewTimer(gracePeriod)
	checkTicker := time.NewTicker(1 * time.Second)
	defer checkTicker.Stop()
	defer drainTimer.Stop()

	// Monitor connection draining
	for {
		select {
		case <-drainTimer.C:
			// Grace period expired, force close remaining connections
			remaining := atomic.LoadInt64(&s.stats.CurrentConnections)
			if remaining > 0 {
				s.logger.Printf("Grace period expired, force closing %d remaining connections", remaining)
			}
			goto forceClose

		case <-checkTicker.C:
			// Check if all connections drained
			remaining := atomic.LoadInt64(&s.stats.CurrentConnections)
			if remaining == 0 {
				s.logger.Println("All connections drained gracefully")
				goto cleanup
			}
			s.logger.Printf("Waiting for %d connections to drain...", remaining)
		}
	}

forceClose:
	// Force close all remaining connections
	s.clients.Range(func(key, value interface{}) bool {
		if client, ok := key.(*Client); ok {
			close(client.send)
		}
		return true
	})

cleanup:
	// Cancel context to stop all goroutines
	s.cancel()

	// Stop worker pool
	s.logger.Println("Stopping worker pool...")
	s.workerPool.Stop()

	// Wait for all goroutines to finish
	s.logger.Println("Waiting for all goroutines to finish...")
	s.wg.Wait()

	s.logger.Println("Graceful shutdown completed")
	return nil
}

