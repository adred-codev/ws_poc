package websocket

import (
	"context"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"odin-ws-server/internal/metrics"
)

const (
	// NumShards for client distribution (power of 2)
	NumShards = 64
	ShardMask = NumShards - 1
)

// Shard represents a partition of clients
type Shard struct {
	clients sync.Map // Lock-free concurrent map
	count   int32    // Atomic counter
}

// HubOptimized is a high-performance hub using sharding and lock-free structures
type HubOptimized struct {
	// Sharded client storage for reduced contention
	shards [NumShards]*Shard

	// Lock-free ring buffer for broadcasts
	broadcastBuffer *RingBuffer

	// Channel-based operations (kept for compatibility)
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte

	// Message pools for zero allocation
	messagePool *MessagePool

	// Metrics
	metrics metrics.MetricsInterface
	logger  *log.Logger

	// Stats (atomic)
	totalConnections int64
	totalMessages    int64

	// Shutdown
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewHubOptimized creates an optimized hub
func NewHubOptimized(metricsInstance metrics.MetricsInterface, logger *log.Logger) *HubOptimized {
	ctx, cancel := context.WithCancel(context.Background())

	// Create shards
	shards := [NumShards]*Shard{}
	for i := 0; i < NumShards; i++ {
		shards[i] = &Shard{}
	}

	// Create optimized hub
	hub := &HubOptimized{
		shards:           shards,
		register:         make(chan *Client, 2000),
		unregister:       make(chan *Client, 2000),
		broadcast:        make(chan []byte, 8192),
		metrics:          metricsInstance,
		logger:           logger,
		totalConnections: 0,
		totalMessages:    0,
		ctx:              ctx,
		cancel:           cancel,
	}

	// Start optimized workers
	go hub.runOptimized(shards)

	return hub
}

// runOptimized runs the optimized hub with multiple workers
func (h *HubOptimized) runOptimized(shards [NumShards]*Shard) {
	h.wg.Add(1)
	defer h.wg.Done()

	// Start broadcast workers (CPU-bound)
	numWorkers := runtime.NumCPU()
	for i := 0; i < numWorkers; i++ {
		go h.broadcastWorker(shards)
	}

	// Main event loop
	for {
		select {
		case <-h.ctx.Done():
			h.logger.Println("Hub shutting down...")
			return

		case client := <-h.register:
			h.registerClientOptimized(client, shards)

		case client := <-h.unregister:
			h.unregisterClientOptimized(client, shards)

		case message := <-h.broadcast:
			h.broadcastMessageOptimized(message, shards)
		}
	}
}

// getShardIndex returns shard index for client (hash-based)
func getShardIndex(client *Client) int {
	// Use pointer address as hash
	addr := uintptr(unsafe.Pointer(client))
	return int(addr & ShardMask)
}

// registerClientOptimized adds client to sharded storage
func (h *HubOptimized) registerClientOptimized(client *Client, shards [NumShards]*Shard) {
	shardIdx := getShardIndex(client)
	shard := shards[shardIdx]

	// Store in lock-free map
	shard.clients.Store(client, true)
	atomic.AddInt32(&shard.count, 1)
	atomic.AddInt64(&h.totalConnections, 1)

	h.metrics.IncrementConnections()
	h.logger.Printf("Client %s connected. Total: %d", client.ID, atomic.LoadInt64(&h.totalConnections))

	// Send connection message using pool
	buf := globalMessagePool.Get(256)
	defer globalMessagePool.Put(buf)

	// Build JSON manually for speed
	buf.Write([]byte(`{"type":"connectionEstablished","clientId":"`))
	buf.Write([]byte(client.ID))
	buf.Write([]byte(`","timestamp":`))
	buf.Write([]byte(time.Now().Format("1641024000000")))
	buf.Write([]byte(`}`))

	select {
	case client.send <- buf.Bytes():
	default:
		// Channel full
	}
}

// unregisterClientOptimized removes client from sharded storage
func (h *HubOptimized) unregisterClientOptimized(client *Client, shards [NumShards]*Shard) {
	shardIdx := getShardIndex(client)
	shard := shards[shardIdx]

	// Remove from lock-free map
	if _, loaded := shard.clients.LoadAndDelete(client); loaded {
		atomic.AddInt32(&shard.count, -1)
		atomic.AddInt64(&h.totalConnections, -1)

		close(client.send)
		h.metrics.DecrementConnections()
		h.logger.Printf("Client %s disconnected. Total: %d", client.ID, atomic.LoadInt64(&h.totalConnections))
	}
}

// broadcastMessageOptimized uses parallel workers for broadcasting
func (h *HubOptimized) broadcastMessageOptimized(message []byte, shards [NumShards]*Shard) {
	// Skip duplicate check for performance (optional)
	atomic.AddInt64(&h.totalMessages, 1)
	h.metrics.IncrementMessagesSent()

	// Parallel broadcast to shards
	var wg sync.WaitGroup
	wg.Add(NumShards)

	for i := 0; i < NumShards; i++ {
		go func(shardIdx int) {
			defer wg.Done()
			shard := shards[shardIdx]

			// Iterate lock-free map
			shard.clients.Range(func(key, value interface{}) bool {
				client := key.(*Client)

				// Non-blocking send
				select {
				case client.send <- message:
				default:
					// Mark for removal if channel full
					go func() {
						h.unregister <- client
					}()
				}
				return true
			})
		}(i)
	}

	wg.Wait()
}

// broadcastWorker processes broadcasts in parallel
func (h *HubOptimized) broadcastWorker(shards [NumShards]*Shard) {
	for {
		select {
		case <-h.ctx.Done():
			return
		case msg := <-h.broadcast:
			h.broadcastMessageOptimized(msg, shards)
		}
	}
}

// GetOptimizedClientCount returns total clients across all shards
func (h *HubOptimized) GetOptimizedClientCount(shards [NumShards]*Shard) int {
	var total int32
	for _, shard := range shards {
		total += atomic.LoadInt32(&shard.count)
	}
	return int(total)
}