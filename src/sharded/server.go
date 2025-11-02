package sharded

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/nats-io/nats.go"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/process"
)

const (
	// WebSocket timing constants (same as monolithic server)
	writeWait  = 5 * time.Second
	pongWait   = 30 * time.Second
	pingPeriod = (pongWait * 9) / 10
)

// ShardedServerConfig configuration for sharded server
type ShardedServerConfig struct {
	Addr           string
	NATSUrl        string
	MaxConnections int
	NumShards      int // Number of shards (default: NumCPU * 2)

	// JetStream configuration
	JSStreamName      string
	JSConsumerName    string
	JSStreamMaxAge    time.Duration
	JSStreamMaxMsgs   int64
	JSStreamMaxBytes  int64
	JSConsumerAckWait time.Duration
}

// ShardedServer is the main server using sharded architecture
type ShardedServer struct {
	config   ShardedServerConfig
	router   *MessageRouter
	logger   *log.Logger
	listener net.Listener
	natsConn *nats.Conn
	natsJS   nats.JetStreamContext
	natsSub  *nats.Subscription

	// Connection management
	clientIDGen    int64         // Atomic counter for client IDs
	connectionsSem chan struct{} // Semaphore for max connections
	shuttingDown   int32         // Atomic flag for graceful shutdown

	// Stats (atomic for thread-safety)
	totalConnections int64
	cpuPercent       float64
	memoryMB         float64

	// Synchronization
	wg sync.WaitGroup
}

// NewShardedServer creates a new sharded WebSocket server
func NewShardedServer(config ShardedServerConfig) (*ShardedServer, error) {
	logger := log.New(os.Stdout, "[ShardedServer] ", log.LstdFlags)

	// Default to 2 shards per CPU core if not specified
	if config.NumShards <= 0 {
		config.NumShards = runtime.NumCPU() * 2
	}

	// Create message router with shards
	router := NewMessageRouter(config.NumShards, logger)

	s := &ShardedServer{
		config:         config,
		router:         router,
		logger:         logger,
		connectionsSem: make(chan struct{}, config.MaxConnections),
		clientIDGen:    0,
	}

	logger.Printf("Created sharded server with %d shards", config.NumShards)

	return s, nil
}

// Start starts the sharded server
func (s *ShardedServer) Start() error {
	// Connect to NATS
	if err := s.connectNATS(); err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}

	// Start HTTP server for WebSocket connections
	if err := s.startHTTPServer(); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	// Start metrics collection
	go s.collectMetrics()

	// Start routing table cleanup
	s.router.StartCleanupTimer()

	// Start stats printer
	go s.printStatsLoop()

	s.logger.Printf("Sharded server started on %s", s.config.Addr)
	return nil
}

// connectNATS establishes connection to NATS and subscribes to JetStream
func (s *ShardedServer) connectNATS() error {
	var err error
	s.natsConn, err = nats.Connect(s.config.NATSUrl,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return err
	}

	s.logger.Printf("Connected to NATS at %s", s.config.NATSUrl)

	// Initialize JetStream
	s.natsJS, err = s.natsConn.JetStream()
	if err != nil {
		s.logger.Printf("❌ FATAL: JetStream init failed: %v", err)
		return fmt.Errorf("failed to initialize jetstream: %w", err)
	}

	// Create or verify JetStream stream
	streamName := s.config.JSStreamName
	streamInfo, err := s.natsJS.StreamInfo(streamName)
	if err != nil {
		// Stream doesn't exist, create it
		s.logger.Printf("📦 Creating JetStream stream: %s", streamName)
		_, err = s.natsJS.AddStream(&nats.StreamConfig{
			Name:      streamName,
			Subjects:  []string{"odin.token.>"},
			Retention: nats.InterestPolicy,
			MaxAge:    s.config.JSStreamMaxAge,
			Storage:   nats.MemoryStorage,
			Replicas:  1,
			Discard:   nats.DiscardOld,
			MaxMsgs:   s.config.JSStreamMaxMsgs,
			MaxBytes:  s.config.JSStreamMaxBytes,
		})
		if err != nil {
			s.logger.Printf("❌ FATAL: Stream creation failed: %v", err)
			return fmt.Errorf("failed to create stream: %w", err)
		}
		s.logger.Printf("✅ JetStream stream created: %s", streamName)
	} else {
		s.logger.Printf("✅ JetStream stream exists: %s (messages: %d)", streamName, streamInfo.State.Msgs)
	}

	// Subscribe to JetStream with durable consumer
	s.natsSub, err = s.natsJS.Subscribe("odin.token.>", s.handleNATSMessage,
		nats.Durable(s.config.JSConsumerName),
		nats.ManualAck(),
		nats.AckWait(s.config.JSConsumerAckWait),
	)
	if err != nil {
		s.natsConn.Close()
		return fmt.Errorf("failed to subscribe to jetstream: %w", err)
	}

	s.logger.Printf("✅ Subscribed to JetStream: odin.token.> (durable, manual ack)")
	return nil
}

// handleNATSMessage processes incoming NATS messages from JetStream
// CRITICAL: This is called from NATS library's goroutine
// Must be fast to avoid blocking NATS message delivery
func (s *ShardedServer) handleNATSMessage(msg *nats.Msg) {
	// Extract channel from subject (e.g., "odin.token.BTC" → "BTC")
	channel := extractChannelFromSubject(msg.Subject)
	if channel == "" {
		// Invalid subject format, NAK for redelivery
		msg.Nak()
		return
	}

	// Route message to relevant shards
	// This is the MAGIC: Router only sends to shards with subscribers
	s.router.Route(channel, msg.Data)

	// ACK message after successful routing
	// JetStream will redeliver if we don't ACK or if we NAK
	if err := msg.Ack(); err != nil {
		s.logger.Printf("Failed to ACK NATS message: %v", err)
	}
}

// extractChannelFromSubject extracts channel name from NATS subject
// Example: "odin.token.BTC" → "BTC", "odin.token.ETH" → "ETH"
// Format: odin.token.{SYMBOL} where SYMBOL can contain slashes (e.g., BTC/USD)
func extractChannelFromSubject(subject string) string {
	// Find "odin.token." prefix and extract everything after
	prefix := "odin.token."
	if len(subject) <= len(prefix) {
		return ""
	}

	// Check if subject starts with prefix
	for i := 0; i < len(prefix); i++ {
		if i >= len(subject) || subject[i] != prefix[i] {
			return ""
		}
	}

	// Return everything after "odin.token."
	return subject[len(prefix):]
}

// startHTTPServer starts the HTTP server for WebSocket connections
func (s *ShardedServer) startHTTPServer() error {
	var err error
	s.listener, err = net.Listen("tcp", s.config.Addr)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/health", s.handleHealth)

	server := &http.Server{
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := server.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			s.logger.Printf("HTTP server error: %v", err)
		}
	}()

	return nil
}

// handleWebSocket handles new WebSocket connections
func (s *ShardedServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Check if shutting down
	if atomic.LoadInt32(&s.shuttingDown) == 1 {
		http.Error(w, "Server is shutting down", http.StatusServiceUnavailable)
		return
	}

	// Try to acquire connection slot
	select {
	case s.connectionsSem <- struct{}{}:
		// Slot acquired
	case <-time.After(5 * time.Second):
		http.Error(w, "Server at capacity", http.StatusServiceUnavailable)
		return
	}

	// Upgrade to WebSocket
	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		<-s.connectionsSem
		s.logger.Printf("Failed to upgrade connection: %v", err)
		return
	}

	// Create client
	clientID := atomic.AddInt64(&s.clientIDGen, 1)
	client := &Client{
		ID:                clientID,
		Conn:              conn,
		Send:              make(chan []byte, 512),
		SeqGen:            NewSequenceGenerator(),
		ReplayBuffer:      NewReplayBuffer(100),
		LastMessageSentAt: time.Now(),
	}

	// Register with router (assigns to shard)
	s.router.RegisterClient(client)
	atomic.AddInt64(&s.totalConnections, 1)

	s.logger.Printf("New client connected: %d (assigned to shard %d)", clientID, client.ShardID)

	// Start read/write pumps
	go s.writePump(client)
	go s.readPump(client)
}

// readPump handles incoming WebSocket messages from client
func (s *ShardedServer) readPump(client *Client) {
	defer func() {
		s.cleanupClient(client)
	}()

	for {
		msg, op, err := wsutil.ReadClientData(client.Conn)
		if err != nil {
			return
		}

		// Handle text messages (subscribe/unsubscribe commands)
		if op == ws.OpText {
			s.handleClientMessage(client, msg)
		}
	}
}

// writePump sends messages to WebSocket client
func (s *ShardedServer) writePump(client *Client) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		s.cleanupClient(client)
	}()

	for {
		select {
		case message, ok := <-client.Send:
			if !ok {
				return
			}

			client.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := wsutil.WriteServerMessage(client.Conn, ws.OpText, message); err != nil {
				return
			}

		case <-ticker.C:
			client.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := wsutil.WriteServerMessage(client.Conn, ws.OpPing, nil); err != nil {
				return
			}
		}
	}
}

// handleClientMessage processes client commands (subscribe/unsubscribe)
func (s *ShardedServer) handleClientMessage(client *Client, data []byte) {
	var msg struct {
		Action   string   `json:"action"`   // "subscribe" or "unsubscribe"
		Channels []string `json:"channels"` // List of channels
	}

	if err := json.Unmarshal(data, &msg); err != nil {
		s.logger.Printf("Invalid client message: %v", err)
		return
	}

	switch msg.Action {
	case "subscribe":
		for _, channel := range msg.Channels {
			s.router.Subscribe(client, channel)
		}

	case "unsubscribe":
		for _, channel := range msg.Channels {
			s.router.Unsubscribe(client, channel)
		}
	}
}

// cleanupClient removes client from server
// CRITICAL: This is called from both readPump and writePump when client disconnects
// Use sync.Once to ensure cleanup only happens once
func (s *ShardedServer) cleanupClient(client *Client) {
	client.CloseOnce.Do(func() {
		// Close connection and channel
		if client.Conn != nil {
			client.Conn.Close()
		}
		close(client.Send)

		// Unregister from shard (must be inside CloseOnce to prevent double-decrement!)
		s.router.UnregisterClient(client)

		// Release connection slot
		<-s.connectionsSem

		s.logger.Printf("Client disconnected: %d", client.ID)
	})
}

// collectMetrics collects CPU and memory metrics
func (s *ShardedServer) collectMetrics() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	proc, _ := process.NewProcess(int32(os.Getpid()))

	for range ticker.C {
		// CPU
		if cpuPercent, err := cpu.Percent(1*time.Second, false); err == nil && len(cpuPercent) > 0 {
			s.cpuPercent = cpuPercent[0]
		}

		// Memory
		if proc != nil {
			if memInfo, err := proc.MemoryInfo(); err == nil {
				s.memoryMB = float64(memInfo.RSS) / 1024 / 1024
			}
		}
	}
}

// printStatsLoop periodically prints server statistics
func (s *ShardedServer) printStatsLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.router.PrintStats()
	}
}

// handleHealth returns server health status
func (s *ShardedServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	stats := s.router.GetStats()
	stats.CPUPercent = s.cpuPercent
	stats.MemoryMB = s.memoryMB

	response := map[string]interface{}{
		"status":       "healthy",
		"architecture": "sharded",
		"shards":       s.config.NumShards,
		"cpu_cores":    runtime.NumCPU(),
		"stats":        stats,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Shutdown gracefully shuts down the server
func (s *ShardedServer) Shutdown(ctx context.Context) error {
	s.logger.Printf("Shutting down sharded server...")

	atomic.StoreInt32(&s.shuttingDown, 1)

	// Stop accepting new connections
	if s.listener != nil {
		s.listener.Close()
	}

	// Unsubscribe from NATS
	if s.natsSub != nil {
		s.natsSub.Unsubscribe()
	}

	// Close NATS connection
	if s.natsConn != nil {
		s.natsConn.Close()
	}

	// Shutdown router (and all shards)
	s.router.Shutdown()

	// Wait for HTTP server to finish
	s.wg.Wait()

	s.logger.Printf("Sharded server shutdown complete")
	return nil
}
