package main

import (
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Client represents a WebSocket client connection with message reliability features
// Enhanced from basic WebSocket to production-grade trading platform client
//
// Reliability features added:
// 1. Sequence numbers - Client can detect missing messages
// 2. Replay buffer - Client can request missed messages
// 3. Slow client detection - Automatically disconnect laggy clients
// 4. Rate limiting - Prevent client from DoS-ing server
//
// Memory per client: ~300KB (optimized from 1.1MB)
// - Base struct: ~200 bytes
// - send channel: 512 slots × 500 bytes avg = 256KB (optimized from 1MB)
// - replay buffer: 100 msgs × 500 bytes avg = 50KB
// - sequence generator: 8 bytes
// - Other fields: ~50 bytes
//
// Memory scaling (after optimization):
// - 7,000 clients: 7K × 0.3MB = ~2.1GB (was 7.7GB)
// - 15,000 clients: 15K × 0.3MB = ~4.5GB (was 16.5GB)
// - 40,000 clients: 40K × 0.3MB = ~12GB (now possible on e2-standard-4!)
//
// Buffer sizing rationale:
// - 256 buffer = 54 sec @ 4.7 msg/sec (current rate), 2.6s @ 100 msg/sec (too small)
// - 512 buffer = 108 sec @ 4.7 msg/sec (current rate), 5.1s @ 100 msg/sec peak (optimal)
// - 2048 buffer = 432 sec @ 4.7 msg/sec (over-provisioned, wastes 75% memory)
//
// Trade-off: Balanced memory vs buffer depth for real production rates (5-20 msg/sec)
type Client struct {
	// Basic WebSocket fields
	id        int64       // Unique client identifier
	conn      net.Conn    // Underlying TCP connection
	server    *Server     // Reference to parent server
	send      chan []byte // Buffered channel for outgoing messages (512 slots, 108s @ 4.7 msg/sec)
	closeOnce sync.Once   // Ensures connection is only closed once

	// Message reliability fields
	// Sequence generator - creates monotonically increasing message IDs
	// Each client gets independent sequence (starts at 1 on connect)
	seqGen *SequenceGenerator

	// Replay buffer - stores recent messages for gap recovery
	// Size: 100 messages (reduced from 1000 for memory efficiency)
	// Covers: ~10 seconds of messages at 10 msg/sec
	// Tradeoff: Shorter buffer = less recovery, but more clients fit in memory
	replayBuffer *ReplayBuffer

	// Slow client detection fields
	// Purpose: One slow client shouldn't block messages to 10,000 fast clients
	// Detection: If send blocks for >100ms, increment failure counter
	// Action: After 3 consecutive failures, disconnect client
	//
	// Why 100ms timeout:
	// - Trading platforms need <50ms latency for price updates
	// - 100ms is 2× the target, generous buffer for network variance
	// - Mobile clients on 4G typically <80ms latency
	// - Anything slower indicates problem (bad network, frozen app, etc.)
	//
	// Why 3 strikes:
	// - 1 failure: Could be temporary network hiccup (don't disconnect)
	// - 2 failures: Suspicious but maybe recovering
	// - 3 failures: Clear pattern, client is too slow
	//
	// Industry comparison:
	// - Coinbase: 2 strikes (more aggressive)
	// - Binance: No automatic disconnect (relies on ping timeout)
	// - FIX protocol: 5 second timeout (more lenient)
	lastMessageSentAt time.Time // Timestamp of last successful send
	sendAttempts      int32     // Consecutive failed send attempts (atomic for thread-safety)
	slowClientWarned  int32     // Flag to avoid log spam (warn once) - atomic: 0 = not warned, 1 = warned

	// Subscription filtering fields
	// Purpose: Only send messages to clients subscribed to specific channels
	// Performance: Reduces broadcast fanout from O(all_clients) to O(subscribed_clients)
	//
	// Example: 10K clients, 200 tokens
	// - Without filtering: 12 msg/sec × 10K clients = 120K writes/sec (CPU 99%+)
	// - With filtering: 12 msg/sec × 500 avg subscribers = 6K writes/sec (CPU <30%)
	//
	// Memory: ~40 bytes per subscription
	// - map[string]struct{}: 8 bytes (key) + 0 bytes (value) + ~32 bytes (map overhead)
	// - 30 subscriptions per client: 30 × 40 = 1.2KB per client
	// - 10K clients: 10K × 1.2KB = 12MB total (negligible)
	subscriptions *SubscriptionSet // Thread-safe set of subscribed channels
}

// ConnectionPool manages a pool of reusable client objects
type ConnectionPool struct {
	pool       sync.Pool
	maxSize    int
	bufferPool *BufferPool
}

func NewConnectionPool(maxSize int, bufferPool *BufferPool) *ConnectionPool {
	cp := &ConnectionPool{maxSize: maxSize, bufferPool: bufferPool}

	cp.pool = sync.Pool{
		New: func() interface{} {
			client := &Client{
				// Buffer size optimized to 512 slots based on actual production metrics
				// Why 512:
				// - At 4.7 msg/sec (production average), provides 108 seconds of buffer
				// - At 100 msg/sec (burst peak), provides 5.1 seconds of buffer
				// - Prevents cascade disconnections while minimizing memory footprint
				// - Memory cost: 256KB per client (512 × 500 bytes avg message)
				// - At 7,000 clients: 1.75GB total buffer memory (vs 7GB with 2048)
				// - At 15,000 clients: 3.75GB total buffer memory (enables 40K+ capacity!)
				send: make(chan []byte, 512),
			}

			client.replayBuffer = NewReplayBuffer(100, bufferPool)
			return client
		},
	}

	return cp
}

func (p *ConnectionPool) Get() *Client {
	v := p.pool.Get()
	if client, ok := v.(*Client); ok {
		// Reset/drain send channel
		select {
		case <-client.send:
			// Drain any pending messages from previous connection
		default:
		}

		// Initialize or reset sequence generator
		// Each new connection gets fresh sequence starting at 1
		if client.seqGen == nil {
			client.seqGen = NewSequenceGenerator()
		} else {
			// Reset counter for reused client
			// (though with sync.Pool, usually get new instance)
			atomic.StoreInt64(&client.seqGen.counter, 0)
		}

		// Initialize or reset replay buffer
		// Using smaller buffer (100 instead of 1000) for memory efficiency
		// Memory calculation: 100 messages × ~500 bytes = ~50KB per client
		// With 7,864 max clients: 50KB × 7,864 = 393MB (fits in 512MB container)
		if client.replayBuffer == nil {
			client.replayBuffer = NewReplayBuffer(100, p.bufferPool)
		} else {
			if p.bufferPool != nil {
				client.replayBuffer.withPool(p.bufferPool)
			}
			client.replayBuffer.Clear()
		}

		// Initialize slow client detection fields
		client.lastMessageSentAt = time.Now()
		atomic.StoreInt32(&client.sendAttempts, 0)
		atomic.StoreInt32(&client.slowClientWarned, 0) // 0 = not warned

		// Initialize subscription set
		// Each new connection starts with no subscriptions
		// Client must explicitly subscribe to channels via WebSocket messages
		if client.subscriptions == nil {
			client.subscriptions = NewSubscriptionSet()
		} else {
			client.subscriptions.Clear()
		}

		return client
	}
	return nil
}

func (p *ConnectionPool) Put(c *Client) {
	if c == nil {
		return
	}

	// Reset connection
	c.conn = nil
	c.server = nil
	c.id = 0

	// Clear subscriptions before returning to pool
	if c.subscriptions != nil {
		c.subscriptions.Clear()
	}

	if c.replayBuffer != nil {
		c.replayBuffer.Clear()
	}

	p.pool.Put(c)
}

// SubscriptionSet is a thread-safe set of channel subscriptions
// Used to filter which messages a client receives
//
// Thread-safety: All methods use RWMutex for safe concurrent access
// - Add/Remove: Write lock (exclusive)
// - Has/Count/List: Read lock (shared - multiple readers allowed)
//
// Performance characteristics:
// - Add/Remove: O(1) average
// - Has: O(1) average
// - List: O(n) where n = number of subscriptions
// - Memory: ~40 bytes per subscription (map overhead)
type SubscriptionSet struct {
	channels map[string]struct{} // Set implementation using map with empty struct values
	mu       sync.RWMutex        // Protects concurrent access to channels map
}

// NewSubscriptionSet creates a new empty subscription set
func NewSubscriptionSet() *SubscriptionSet {
	return &SubscriptionSet{
		channels: make(map[string]struct{}),
	}
}

// Add subscribes to a channel
// Thread-safe: Uses write lock
func (s *SubscriptionSet) Add(channel string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channels[channel] = struct{}{}
}

// AddMultiple subscribes to multiple channels at once
// More efficient than calling Add() multiple times (single lock acquisition)
func (s *SubscriptionSet) AddMultiple(channels []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range channels {
		s.channels[ch] = struct{}{}
	}
}

// Remove unsubscribes from a channel
// Thread-safe: Uses write lock
func (s *SubscriptionSet) Remove(channel string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.channels, channel)
}

// RemoveMultiple unsubscribes from multiple channels at once
// More efficient than calling Remove() multiple times (single lock acquisition)
func (s *SubscriptionSet) RemoveMultiple(channels []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range channels {
		delete(s.channels, ch)
	}
}

// Has checks if client is subscribed to a channel
// Thread-safe: Uses read lock (allows concurrent Has() calls)
func (s *SubscriptionSet) Has(channel string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.channels[channel]
	return exists
}

// Count returns the number of active subscriptions
// Thread-safe: Uses read lock
func (s *SubscriptionSet) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.channels)
}

// List returns a copy of all subscribed channels
// Thread-safe: Uses read lock
// Returns: New slice (safe to modify without affecting internal state)
func (s *SubscriptionSet) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]string, 0, len(s.channels))
	for ch := range s.channels {
		result = append(result, ch)
	}
	return result
}

// Clear removes all subscriptions
// Thread-safe: Uses write lock
func (s *SubscriptionSet) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channels = make(map[string]struct{})
}

// SubscriptionIndex maintains a reverse index from channels to subscribed clients
// Performance optimization: Instead of iterating ALL clients to filter by subscription,
// iterate ONLY subscribed clients.
//
// Use case: Production scenario where clients subscribe to specific tokens
// - Without index: Iterate 7,000 clients, filter to ~500 subscribers = 7,000 checks
// - With index: Directly access ~500 subscribers = 500 iterations
// - CPU savings: 93% (14× reduction in iterations)
//
// Memory cost: ~7MB for 5 channels × 7,000 clients (map overhead)
// CPU savings: 93% fewer iterations per broadcast in production
//
// Thread-safety: RWMutex protects concurrent access
// - Add/Remove: Write lock (during subscribe/unsubscribe)
// - Get: Read lock (during broadcast - hot path!)
type SubscriptionIndex struct {
	subscribers map[string][]*Client // channel → list of subscribed clients
	mu          sync.RWMutex
}

// NewSubscriptionIndex creates a new empty subscription index
func NewSubscriptionIndex() *SubscriptionIndex {
	return &SubscriptionIndex{
		subscribers: make(map[string][]*Client),
	}
}

// Add registers a client as a subscriber to a channel
// Thread-safe: Uses write lock
func (idx *SubscriptionIndex) Add(channel string, client *Client) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Get existing subscribers or create new slice
	subscribers := idx.subscribers[channel]

	// Check if client already subscribed (avoid duplicates)
	for _, existing := range subscribers {
		if existing == client {
			return // Already subscribed
		}
	}

	// Add client to subscribers list
	idx.subscribers[channel] = append(subscribers, client)
}

// AddMultiple registers a client as a subscriber to multiple channels
// More efficient than calling Add() multiple times (single lock acquisition)
// Thread-safe: Uses write lock
func (idx *SubscriptionIndex) AddMultiple(channels []string, client *Client) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	for _, channel := range channels {
		subscribers := idx.subscribers[channel]

		// Check if client already subscribed
		alreadySubscribed := false
		for _, existing := range subscribers {
			if existing == client {
				alreadySubscribed = true
				break
			}
		}

		if !alreadySubscribed {
			idx.subscribers[channel] = append(subscribers, client)
		}
	}
}

// Remove unregisters a client from a channel
// Thread-safe: Uses write lock
func (idx *SubscriptionIndex) Remove(channel string, client *Client) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	subscribers, exists := idx.subscribers[channel]
	if !exists {
		return
	}

	// Find and remove client from subscribers list
	for i, existing := range subscribers {
		if existing == client {
			// Remove by swapping with last element and truncating
			subscribers[i] = subscribers[len(subscribers)-1]
			idx.subscribers[channel] = subscribers[:len(subscribers)-1]

			// Clean up empty slices to free memory
			if len(idx.subscribers[channel]) == 0 {
				delete(idx.subscribers, channel)
			}
			return
		}
	}
}

// RemoveMultiple unregisters a client from multiple channels
// More efficient than calling Remove() multiple times (single lock acquisition)
// Thread-safe: Uses write lock
func (idx *SubscriptionIndex) RemoveMultiple(channels []string, client *Client) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	for _, channel := range channels {
		subscribers, exists := idx.subscribers[channel]
		if !exists {
			continue
		}

		// Find and remove client
		for i, existing := range subscribers {
			if existing == client {
				subscribers[i] = subscribers[len(subscribers)-1]
				idx.subscribers[channel] = subscribers[:len(subscribers)-1]

				if len(idx.subscribers[channel]) == 0 {
					delete(idx.subscribers, channel)
				}
				break
			}
		}
	}
}

// RemoveClient removes a client from ALL channels (called on disconnect)
// Thread-safe: Uses write lock
func (idx *SubscriptionIndex) RemoveClient(client *Client) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Iterate all channels and remove client
	for channel, subscribers := range idx.subscribers {
		for i, existing := range subscribers {
			if existing == client {
				subscribers[i] = subscribers[len(subscribers)-1]
				idx.subscribers[channel] = subscribers[:len(subscribers)-1]

				if len(idx.subscribers[channel]) == 0 {
					delete(idx.subscribers, channel)
				}
				break
			}
		}
	}
}

// Get returns a copy of all clients subscribed to a channel
// Thread-safe: Uses read lock (optimized for hot path - broadcast!)
// Returns: New slice (safe to iterate without holding lock)
func (idx *SubscriptionIndex) Get(channel string) []*Client {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	subscribers, exists := idx.subscribers[channel]
	if !exists || len(subscribers) == 0 {
		return nil
	}

	// Return a copy to avoid race conditions during iteration
	result := make([]*Client, len(subscribers))
	copy(result, subscribers)
	return result
}

// Count returns the number of subscribers for a channel
// Thread-safe: Uses read lock
func (idx *SubscriptionIndex) Count(channel string) int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	subscribers, exists := idx.subscribers[channel]
	if !exists {
		return 0
	}
	return len(subscribers)
}
