package main

import (
	"sync"
)

// ReplayBuffer stores recent messages per client for replay on request
// This implements the industry-standard message recovery pattern used by:
// - FIX protocol: Gap fill and resend requests
// - NATS JetStream: Message replay from stream
// - Kafka: Consumer offset seeking
// - Trading platforms: Recovery from network hiccups
//
// Purpose:
// 1. Client reconnection - "Give me everything since seq 1000"
// 2. Gap detection - "I got seq 100, then 150, replay 101-149"
// 3. Network recovery - Cheaper than full reconnect and resubscribe
// 4. Regulatory compliance - Prove we delivered critical messages
//
// Memory Cost Analysis (Important for capacity planning):
// - Average message size: ~500 bytes (price update JSON)
// - Buffer size: 1000 messages
// - Memory per client: 500 bytes × 1000 = 500KB
// - With 10,000 clients: 500KB × 10,000 = 5GB
// - With 7,864 clients (our limit): 500KB × 7,864 = 3.9GB
//
// Mitigation strategies:
// 1. Use smaller buffer (100 messages = 50KB per client)
// 2. Compress old messages (gzip reduces by 80%)
// 3. Share buffer across clients for same symbol (dedupe)
// 4. Offload to Redis after 10 seconds (hot vs cold storage)
//
// For initial deployment: 1000 messages is safe with 512MB limit
// because we limit connections to ~7K (memory aware)
type ReplayBuffer struct {
	// Circular buffer of recent messages
	// We use slice instead of ring buffer for simplicity
	// Production optimization: Implement proper ring buffer for O(1) add
	messages []*MessageEnvelope

	// Maximum messages to keep
	// Trading platforms typically buffer:
	// - Retail platforms: 100-1000 messages (~10 seconds @ 10msg/sec)
	// - Institutional: 10,000-100,000 messages (~1 hour of data)
	// - Our choice: 1000 (good balance of recovery vs memory)
	maxSize int

	// Read-write mutex for thread safety
	// Multiple scenarios:
	// - broadcast() writes (1 writer, many buffers)
	// - handleClientMessage() reads for replay (many concurrent readers)
	// - RWMutex allows multiple concurrent reads, exclusive writes
	// - Performance: Reads don't block each other (important for replay)
	mu sync.RWMutex
}

// NewReplayBuffer creates a buffer with specified capacity
// Typical usage:
//   buffer := NewReplayBuffer(1000)  // 1000 messages per client
//
// Size recommendations by use case:
//   100   - Casual trading app (covers network hiccups)
//   1000  - Professional trading (covers 1-2 min outage)
//   10000 - Institutional trading (covers longer outages)
func NewReplayBuffer(maxSize int) *ReplayBuffer {
	return &ReplayBuffer{
		messages: make([]*MessageEnvelope, 0, maxSize),
		maxSize:  maxSize,
	}
}

// Add appends message to buffer, evicting oldest if full
// Called from broadcast() after sending message
// Thread-safe: Multiple connections can add concurrently
//
// Implementation note:
// Current: O(n) eviction using slice shift
// Production optimization: O(1) using ring buffer with head/tail pointers
//
// Example ring buffer:
//   messages := make([]*MessageEnvelope, maxSize)
//   head := 0  // Next write position
//   tail := 0  // Oldest message
//   count := 0 // Number of messages
//
//   To add: messages[head] = msg; head = (head + 1) % maxSize
//
// Why we use simple version:
//   - Easier to understand and debug
//   - Performance impact minimal (happens in background goroutine)
//   - Can optimize later if profiling shows bottleneck
func (rb *ReplayBuffer) Add(msg *MessageEnvelope) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	// If buffer full, remove oldest message (FIFO)
	if len(rb.messages) >= rb.maxSize {
		// Shift all elements left (oldest message at index 0 drops off)
		// This is O(n) - in production, use ring buffer for O(1)
		//
		// Memory note: This creates garbage (old slice header)
		// but Go GC will clean it up efficiently
		rb.messages = rb.messages[1:]
	}

	// Append new message to end
	// Newest messages always at the end of slice
	rb.messages = append(rb.messages, msg)
}

// GetRange returns messages with sequence numbers from fromSeq to toSeq (inclusive)
// Used when client detects gap and requests specific range
//
// Client-side usage pattern:
//   lastSeq = 100
//   received = 150
//   // Detected gap!
//   ws.send({"type": "replay", "data": {"from": 101, "to": 149}})
//
// Server calls this to find messages in range
// Returns:
//   - Empty slice if no messages in range (already evicted from buffer)
//   - Partial slice if some messages evicted (client needs full reconnect)
//   - Full slice if all messages available (normal case)
//
// Performance:
//   - O(n) where n = buffer size
//   - For 1000 message buffer: ~1 microsecond on modern CPU
//   - Not a bottleneck (replay requests are rare)
//
// Production optimization:
//   - Use binary search to find start position (O(log n))
//   - Index by sequence number (O(1) lookup, more memory)
func (rb *ReplayBuffer) GetRange(fromSeq, toSeq int64) []*MessageEnvelope {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	result := make([]*MessageEnvelope, 0)

	// Linear scan through buffer
	// Messages are already sorted by sequence (monotonically increasing)
	for _, msg := range rb.messages {
		if msg.Seq >= fromSeq && msg.Seq <= toSeq {
			result = append(result, msg)
		}
		// Early exit optimization: Once we pass toSeq, no more matches
		if msg.Seq > toSeq {
			break
		}
	}

	return result
}

// GetSince returns all messages after a sequence number
// Used during reconnection when client knows last received sequence
//
// Reconnection flow:
//   1. Client disconnects (network issue, server restart)
//   2. Client reconnects immediately
//   3. Client sends: {"type": "replay", "data": {"since": 1000}}
//   4. Server sends all messages seq > 1000
//   5. Client is caught up, resumes normal operation
//
// This is faster than:
//   - Full page refresh (loses client state)
//   - Resubscribing to all symbols (many round trips)
//   - Fetching from database (slow, may not have latest data)
//
// Edge cases:
//   - since=0: Returns all buffered messages (full replay)
//   - since > newest: Returns empty (client is already caught up)
//   - since < oldest: Returns what we have (client needs full refresh)
//
// Client should check:
//   if (replay.length > 0 && replay[0].seq != lastSeq + 1) {
//     // Gap detected - some messages evicted from buffer
//     // Need full reconnect and resync
//     fullReconnect()
//   }
func (rb *ReplayBuffer) GetSince(sinceSeq int64) []*MessageEnvelope {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	result := make([]*MessageEnvelope, 0)

	for _, msg := range rb.messages {
		if msg.Seq > sinceSeq {
			result = append(result, msg)
		}
	}

	return result
}

// Clear empties the buffer completely
// Used on client disconnect to free memory
//
// Important: We don't actually need to clear because ConnectionPool
// reuses Client objects. But explicit clear:
// 1. Makes memory usage predictable
// 2. Prevents accidentally sending old client's messages to new client
// 3. Helps garbage collector (removes all references)
//
// Production consideration:
// Instead of clearing, could reuse buffer for next client
// (saves allocation). But risky - could leak messages between clients.
// For trading platform, safety > performance here.
func (rb *ReplayBuffer) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	// Truncate slice to length 0 but keep capacity
	// This keeps allocated memory for reuse
	// To fully free memory: rb.messages = nil
	rb.messages = rb.messages[:0]
}
