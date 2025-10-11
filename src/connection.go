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
// Memory per client: ~1.1MB
// - Base struct: ~200 bytes
// - send channel: 2048 slots × 500 bytes avg = 1MB (largest - increased for scale)
// - replay buffer: 100 msgs × 500 bytes avg = 50KB
// - sequence generator: 8 bytes
// - Other fields: ~100 bytes
//
// Memory scaling:
// - 500 clients: 500 × 1.1MB = ~550MB (fits comfortably in 3.5GB container)
// - 1000 clients: 1000 × 1.1MB = ~1.1GB (safe)
// - 2000 clients: 2000 × 1.1MB = ~2.2GB (approaching limit)
// - 3000 clients: 3000 × 1.1MB = ~3.3GB (near 3.5GB limit)
//
// Buffer sizing rationale:
// - 256 buffer = 12.8 sec @ 20 broadcasts/sec (too small, caused cascade disconnects)
// - 2048 buffer = 102 sec @ 20 broadcasts/sec (provides ample recovery time)
//
// Trade-off: Higher memory usage but prevents slow client disconnections at scale
type Client struct {
	// Basic WebSocket fields
	id     int64       // Unique client identifier
	conn   net.Conn    // Underlying TCP connection
	server *Server     // Reference to parent server
	send   chan []byte // Buffered channel for outgoing messages (2048 deep)
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
	sendAttempts      int32     // Consecutive failed send attempts (atomic for thread-safety)
	slowClientWarned  bool      // Flag to avoid log spam (warn once)
}

// ConnectionPool manages a pool of reusable client objects
type ConnectionPool struct {
	pool    sync.Pool
	maxSize int
	current int64
}

func NewConnectionPool(maxSize int) *ConnectionPool {
	return &ConnectionPool{
		maxSize: maxSize,
		pool: sync.Pool{
			New: func() interface{} {
				return &Client{
					// Buffer size increased from 256 to 2048 for high-scale scenarios
					// Why 2048:
					// - At 20 broadcasts/sec, provides 102 seconds of buffer (vs 12.8s with 256)
					// - Prevents cascade disconnections when clients temporarily slow
					// - Memory cost: 1MB per client (2048 × 500 bytes avg message)
					// - At 1000 clients: 1GB total buffer memory (acceptable)
					// - At 500 clients: 500MB total buffer memory (very safe)
					send: make(chan []byte, 2048),
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
