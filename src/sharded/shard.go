package sharded

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// Shard is a single-threaded partition of clients and subscriptions
// CRITICAL DESIGN: All state is accessed by ONE goroutine (Run loop)
// This eliminates ALL lock contention for subscription lookups and broadcasts
type Shard struct {
	// Identity
	ID      int
	CPUCore int // Which CPU core this shard prefers (for affinity)

	// State - ALL accessed single-threaded by Run() loop
	// NO LOCKS NEEDED because only one goroutine accesses these
	clients       map[int64]*Client        // clientID → Client
	subscriptions map[string][]*Client     // channel → list of subscribers
	clientSubs    map[int64]map[string]bool // clientID → set of subscribed channels (for cleanup)

	// Communication channels - commands sent FROM other goroutines TO this shard
	broadcast   chan BroadcastMsg  // Messages to broadcast to subscribers
	subscribe   chan SubscribeCmd  // Subscribe commands
	unsubscribe chan UnsubscribeCmd // Unsubscribe commands
	register    chan *Client       // New client registration
	unregister  chan *Client       // Client disconnection

	// Metrics - atomic for cross-shard reading
	clientCount     int64 // Current number of clients in this shard
	messagesSent    int64 // Total messages successfully sent
	messagesDropped int64 // Messages dropped due to full buffers

	// Buffer pool for message serialization (per-shard to avoid contention)
	bufferPool *sync.Pool

	// Logger
	logger *log.Logger

	// Shutdown
	ctx    context.Context
	cancel context.CancelFunc
}

// NewShard creates a new shard with the given ID and CPU affinity
func NewShard(id, cpuCore int, logger *log.Logger) *Shard {
	ctx, cancel := context.WithCancel(context.Background())

	return &Shard{
		ID:            id,
		CPUCore:       cpuCore,
		clients:       make(map[int64]*Client, 10000),
		subscriptions: make(map[string][]*Client),
		clientSubs:    make(map[int64]map[string]bool),
		broadcast:     make(chan BroadcastMsg, 2048),  // Large buffer for high throughput
		subscribe:     make(chan SubscribeCmd, 512),   // Medium buffer
		unsubscribe:   make(chan UnsubscribeCmd, 512), // Medium buffer
		register:      make(chan *Client, 256),        // Medium buffer
		unregister:    make(chan *Client, 256),        // Medium buffer
		clientCount:   0,
		messagesSent:  0,
		logger:        logger,
		ctx:           ctx,
		cancel:        cancel,
		bufferPool: &sync.Pool{
			New: func() interface{} {
				b := make([]byte, 0, 2048)
				return &b
			},
		},
	}
}

// Run is the main event loop for this shard
// CRITICAL: This is single-threaded - only ONE goroutine runs this
// All state access happens here, eliminating need for locks
func (s *Shard) Run() {
	// Pin this goroutine to a specific OS thread for better cache locality
	// This keeps the shard's data in the same CPU's L1/L2 cache
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	s.logger.Printf("[Shard %d] Starting on CPU core %d", s.ID, s.CPUCore)

	// Stats ticker for periodic logging
	statsTicker := time.NewTicker(30 * time.Second)
	defer statsTicker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Printf("[Shard %d] Shutting down", s.ID)
			return

		case client := <-s.register:
			s.handleRegister(client)

		case client := <-s.unregister:
			s.handleUnregister(client)

		case cmd := <-s.subscribe:
			s.handleSubscribe(cmd)

		case cmd := <-s.unsubscribe:
			s.handleUnsubscribe(cmd)

		case msg := <-s.broadcast:
			// CRITICAL: This is where the magic happens
			// Broadcast runs single-threaded, accessing subscriptions map
			// with ZERO lock contention
			s.handleBroadcast(msg)

		case <-statsTicker.C:
			s.logStats()
		}
	}
}

// handleRegister adds a new client to this shard
// Single-threaded access - no locks needed
func (s *Shard) handleRegister(client *Client) {
	s.clients[client.ID] = client
	s.clientSubs[client.ID] = make(map[string]bool)
	atomic.AddInt64(&s.clientCount, 1)

	s.logger.Printf("[Shard %d] Registered client %d (total: %d)",
		s.ID, client.ID, atomic.LoadInt64(&s.clientCount))
}

// handleUnregister removes a client from this shard
// Single-threaded access - no locks needed
func (s *Shard) handleUnregister(client *Client) {
	// Remove from clients map
	delete(s.clients, client.ID)

	// Remove from all subscribed channels
	if channels, exists := s.clientSubs[client.ID]; exists {
		for channel := range channels {
			s.removeClientFromChannel(client, channel)
		}
		delete(s.clientSubs, client.ID)
	}

	atomic.AddInt64(&s.clientCount, -1)

	s.logger.Printf("[Shard %d] Unregistered client %d (total: %d)",
		s.ID, client.ID, atomic.LoadInt64(&s.clientCount))
}

// handleSubscribe subscribes a client to a channel
// Single-threaded access - no locks needed
func (s *Shard) handleSubscribe(cmd SubscribeCmd) {
	// Add to channel's subscriber list
	subscribers := s.subscriptions[cmd.Channel]

	// Check if already subscribed (avoid duplicates)
	for _, existing := range subscribers {
		if existing.ID == cmd.Client.ID {
			return // Already subscribed
		}
	}

	s.subscriptions[cmd.Channel] = append(subscribers, cmd.Client)

	// Track client's subscriptions for cleanup
	if s.clientSubs[cmd.Client.ID] == nil {
		s.clientSubs[cmd.Client.ID] = make(map[string]bool)
	}
	s.clientSubs[cmd.Client.ID][cmd.Channel] = true

	s.logger.Printf("[Shard %d] Client %d subscribed to %s (subscribers: %d)",
		s.ID, cmd.Client.ID, cmd.Channel, len(s.subscriptions[cmd.Channel]))
}

// handleUnsubscribe unsubscribes a client from a channel
// Single-threaded access - no locks needed
func (s *Shard) handleUnsubscribe(cmd UnsubscribeCmd) {
	s.removeClientFromChannel(cmd.Client, cmd.Channel)

	// Remove from client's subscription tracking
	if subs, exists := s.clientSubs[cmd.Client.ID]; exists {
		delete(subs, cmd.Channel)
	}

	s.logger.Printf("[Shard %d] Client %d unsubscribed from %s",
		s.ID, cmd.Client.ID, cmd.Channel)
}

// removeClientFromChannel removes a client from a channel's subscriber list
func (s *Shard) removeClientFromChannel(client *Client, channel string) {
	subscribers, exists := s.subscriptions[channel]
	if !exists {
		return
	}

	// Find and remove client
	for i, existing := range subscribers {
		if existing.ID == client.ID {
			// Swap with last and truncate (efficient removal)
			subscribers[i] = subscribers[len(subscribers)-1]
			s.subscriptions[channel] = subscribers[:len(subscribers)-1]

			// Clean up empty channel
			if len(s.subscriptions[channel]) == 0 {
				delete(s.subscriptions, channel)
			}
			return
		}
	}
}

// handleBroadcast sends a message to all subscribers in THIS shard
// CRITICAL: This is the hot path - runs single-threaded with zero locks
func (s *Shard) handleBroadcast(msg BroadcastMsg) {
	// Get subscribers for this channel (no lock needed!)
	subscribers, exists := s.subscriptions[msg.Channel]
	if !exists || len(subscribers) == 0 {
		return // No subscribers in this shard
	}

	// Send to each subscriber (non-blocking)
	// Note: We serialize per-client because each client needs a unique sequence number
	sent := 0
	dropped := 0

	for _, client := range subscribers {
		// Add sequence number (per-client, managed by shard)
		seq := client.SeqGen.Next()

		// Create final message with sequence
		finalEnvelope := MessageEnvelope{
			Type:      "price:update",
			Channel:   msg.Channel,
			Data:      json.RawMessage(msg.Data),
			Sequence:  seq,
			Timestamp: msg.Timestamp,
			Priority:  1,
		}

		finalData, _ := json.Marshal(finalEnvelope)

		// Store in replay buffer
		client.ReplayBuffer.Add(finalData)

		// Non-blocking send
		select {
		case client.Send <- finalData:
			// Success
			sent++
			client.SendAttempts = 0
			client.LastMessageSentAt = time.Now()

		default:
			// Buffer full - client is slow
			dropped++
			client.SendAttempts++

			// Log first failure
			if client.SendAttempts == 1 && !client.SlowClientWarned {
				s.logger.Printf("[Shard %d] Client %d is slow (buffer full)", s.ID, client.ID)
				client.SlowClientWarned = true
			}

			// Disconnect after 3 consecutive failures
			if client.SendAttempts >= 3 {
				s.logger.Printf("[Shard %d] Disconnecting slow client %d (3 failed sends)",
					s.ID, client.ID)

				// Close client connection (will trigger unregister via readPump/writePump)
				client.CloseOnce.Do(func() {
					if client.Conn != nil {
						client.Conn.Close()
					}
				})
			}
		}
	}

	// Update metrics
	atomic.AddInt64(&s.messagesSent, int64(sent))
	atomic.AddInt64(&s.messagesDropped, int64(dropped))
}

// logStats prints periodic statistics
func (s *Shard) logStats() {
	clients := atomic.LoadInt64(&s.clientCount)
	sent := atomic.LoadInt64(&s.messagesSent)
	dropped := atomic.LoadInt64(&s.messagesDropped)

	s.logger.Printf("[Shard %d] Stats: clients=%d, sent=%d, dropped=%d, channels=%d",
		s.ID, clients, sent, dropped, len(s.subscriptions))
}

// GetStats returns current shard statistics
func (s *Shard) GetStats() ShardStats {
	// Count subscribers per channel
	subCounts := make(map[string]int)
	for channel, subscribers := range s.subscriptions {
		subCounts[channel] = len(subscribers)
	}

	return ShardStats{
		ID:              s.ID,
		ClientCount:     len(s.clients),
		MessagesSent:    atomic.LoadInt64(&s.messagesSent),
		MessagesDropped: atomic.LoadInt64(&s.messagesDropped),
		SubscriberCount: subCounts,
	}
}

// Shutdown gracefully shuts down the shard
func (s *Shard) Shutdown() {
	s.logger.Printf("[Shard %d] Shutdown initiated", s.ID)
	s.cancel()
}

// MessageEnvelope wraps messages with metadata
type MessageEnvelope struct {
	Type      string          `json:"type"`
	Channel   string          `json:"channel"`
	Data      json.RawMessage `json:"data"`
	Sequence  int64           `json:"seq"`
	Timestamp int64           `json:"ts"`
	Priority  int             `json:"priority"`
}

// Helper function to format channel stats
func (s *Shard) GetChannelStats() string {
	var stats string
	for channel, subscribers := range s.subscriptions {
		stats += fmt.Sprintf("  %s: %d subscribers\n", channel, len(subscribers))
	}
	return stats
}
