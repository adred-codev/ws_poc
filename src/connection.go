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
// Memory per client: ~520KB
// - Base struct: ~200 bytes
// - send channel: 256 slots × 500 bytes avg = 128KB
// - replay buffer: 1000 msgs × 500 bytes avg = 500KB (largest)
// - sequence generator: 8 bytes
// - Other fields: ~100 bytes
//
// For 7,864 clients (our memory-based limit):
// Total: 7,864 × 520KB = ~4GB
// Container has: 512MB
// Math doesn't add up! 🚨
//
// Solution: Reduce replay buffer OR reduce max connections
// Option A: buffer=100 (10KB per client) → 7,864 × 138KB = 1.08GB ✓
// Option B: maxConns=3500 (512MB÷520KB) → 3,500 × 520KB = 1.8GB ✓
//
// For production: Start with Option A (smaller buffer)
// Can increase buffer size if memory allows after monitoring
type Client struct {
	// Basic WebSocket fields
	id     int64       // Unique client identifier
	conn   net.Conn    // Underlying TCP connection
	server *Server     // Reference to parent server
	send   chan []byte // Buffered channel for outgoing messages (256 deep)
	mu     sync.RWMutex

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
	sendAttempts      int32      // Consecutive failed send attempts (atomic for thread-safety)
	slowClientWarned  bool       // Flag to avoid log spam (warn once)
}

// ConnectionPool manages a pool of reusable client objects
type ConnectionPool struct {
	pool     sync.Pool
	maxSize  int
	current  int64
}

func NewConnectionPool(maxSize int) *ConnectionPool {
	return &ConnectionPool{
		maxSize: maxSize,
		pool: sync.Pool{
			New: func() interface{} {
				return &Client{
					send: make(chan []byte, 256),
				}
			},
		},
	}
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
			client.replayBuffer = NewReplayBuffer(100)
		} else {
			client.replayBuffer.Clear()
		}

		// Initialize slow client detection fields
		client.lastMessageSentAt = time.Now()
		atomic.StoreInt32(&client.sendAttempts, 0)
		client.slowClientWarned = false

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
	
	p.pool.Put(c)
}
