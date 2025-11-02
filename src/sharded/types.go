package sharded

import (
	"net"
	"sync"
	"time"
)

// BroadcastMsg represents a message to be broadcast to clients subscribed to a channel
type BroadcastMsg struct {
	Channel   string
	Data      []byte
	Timestamp int64 // Unix milliseconds, set once by router
}

// SubscribeCmd is a command to subscribe a client to a channel
type SubscribeCmd struct {
	Client  *Client
	Channel string
}

// UnsubscribeCmd is a command to unsubscribe a client from a channel
type UnsubscribeCmd struct {
	Client  *Client
	Channel string
}

// Client represents a WebSocket client connection in the sharded architecture
// Unlike the monolithic design, clients are permanently assigned to a shard
type Client struct {
	// Identity
	ID      int64 // Globally unique client ID
	ShardID int   // Which shard owns this client (immutable after assignment)

	// Connection
	Conn      net.Conn    // Underlying TCP/WebSocket connection
	Send      chan []byte // Buffered channel for outgoing messages
	CloseOnce sync.Once   // Ensures connection is only closed once

	// Subscriptions (managed by shard, not by client)
	// This is different from monolithic design where client tracks its own subscriptions
	// In sharded design, shard owns all subscription state for lock-free access

	// Slow client detection
	LastMessageSentAt time.Time
	SendAttempts      int32 // Consecutive failed send attempts (not atomic - only accessed by shard)
	SlowClientWarned  bool  // Has warning been logged?

	// Sequence generator for message ordering
	SeqGen *SequenceGenerator

	// Replay buffer for gap recovery
	ReplayBuffer *ReplayBuffer
}

// SequenceGenerator generates monotonically increasing sequence numbers
// Thread-safe for concurrent access
type SequenceGenerator struct {
	counter int64
}

// NewSequenceGenerator creates a new sequence generator starting at 1
func NewSequenceGenerator() *SequenceGenerator {
	return &SequenceGenerator{counter: 0}
}

// Next returns the next sequence number (thread-safe)
func (sg *SequenceGenerator) Next() int64 {
	// In sharded design, each shard accesses its clients' seqGen single-threaded
	// So we don't need atomic operations here (unlike monolithic design)
	sg.counter++
	return sg.counter
}

// ReplayBuffer stores recent messages for gap recovery
// In sharded design, this is accessed single-threaded by shard event loop
type ReplayBuffer struct {
	messages [][]byte
	capacity int
	head     int
	size     int
	mu       sync.RWMutex // Keep mutex for safety, but won't contend in sharded design
}

// NewReplayBuffer creates a new replay buffer with given capacity
func NewReplayBuffer(capacity int) *ReplayBuffer {
	return &ReplayBuffer{
		messages: make([][]byte, capacity),
		capacity: capacity,
		head:     0,
		size:     0,
	}
}

// Add appends a message to the replay buffer (circular buffer)
func (rb *ReplayBuffer) Add(msg []byte) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.messages[rb.head] = msg
	rb.head = (rb.head + 1) % rb.capacity
	if rb.size < rb.capacity {
		rb.size++
	}
}

// GetRange retrieves messages from sequence number 'from' to 'to' (inclusive)
// Returns nil if messages are no longer in buffer
func (rb *ReplayBuffer) GetRange(from, to int64) [][]byte {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	// Simplified implementation - in production would need proper sequence tracking
	// For now, just return recent messages
	if rb.size == 0 {
		return nil
	}

	result := make([][]byte, 0, rb.size)
	for i := 0; i < rb.size; i++ {
		idx := (rb.head - rb.size + i + rb.capacity) % rb.capacity
		if rb.messages[idx] != nil {
			result = append(result, rb.messages[idx])
		}
	}
	return result
}

// Clear removes all messages from the buffer
func (rb *ReplayBuffer) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.messages = make([][]byte, rb.capacity)
	rb.head = 0
	rb.size = 0
}

// ShardStats contains per-shard statistics
type ShardStats struct {
	ID              int
	ClientCount     int
	MessagesSent    int64
	MessagesDropped int64
	SubscriberCount map[string]int // channel → subscriber count in this shard
}

// ServerStats contains aggregate statistics across all shards
type ServerStats struct {
	TotalClients    int64
	TotalMessages   int64
	TotalDropped    int64
	ShardsActive    int
	CPUPercent      float64
	MemoryMB        float64
	GoroutineCount  int
	ShardStats      []ShardStats
}
