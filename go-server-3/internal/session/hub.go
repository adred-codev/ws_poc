package session

import (
	"context"
	"net"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"

	"odin-ws-server-3/internal/config"
	"odin-ws-server-3/internal/metrics"
)

type Connection struct {
	ID        uint64
	Conn      net.Conn
	SendQueue chan []byte
}

type shard struct {
	clients sync.Map // map[uint64]*Connection
	count   int32
}

// Hub manages WebSocket connections and broadcasts.
type Hub struct {
	cfg             config.WebSocketConfig
	shards          []shard
	nextConnection  uint64
	metrics         *metrics.Registry
	metricsConnGauge prometheus.Gauge

	broadcastQueue chan []byte
	workers        int
}

func NewHub(cfg config.WebSocketConfig, metricsRegistry *metrics.Registry) *Hub {
	shardCount := cfg.ShardCount
	if shardCount <= 0 {
		shardCount = 64
	}
	shards := make([]shard, shardCount)

	queueSize := cfg.BroadcastQueueSize
	if queueSize <= 0 {
		queueSize = 1024
	}

	workers := cfg.BroadcastWorkers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	return &Hub{
		cfg:             cfg,
		shards:          shards,
		metrics:         metricsRegistry,
		metricsConnGauge: metricsRegistry.Connections.ActiveConnections,
		broadcastQueue: make(chan []byte, queueSize),
		workers:        workers,
	}
}

func (h *Hub) StartBroadcastWorkers() {
	for i := 0; i < h.workers; i++ {
		go h.broadcastWorker()
	}
}

func (h *Hub) Register(conn net.Conn) *Connection {
	id := atomic.AddUint64(&h.nextConnection, 1)
	shard := h.pickShard(id)

	c := &Connection{
		ID:        id,
		Conn:      conn,
		SendQueue: make(chan []byte, h.cfg.SendChannelSize),
	}

	shard.clients.Store(id, c)
	atomic.AddInt32(&shard.count, 1)
	h.metricsConnGauge.Inc()
	return c
}

func (h *Hub) Unregister(c *Connection) {
	if c == nil {
		return
	}
	shard := h.pickShard(c.ID)
	if _, ok := shard.clients.LoadAndDelete(c.ID); ok {
		atomic.AddInt32(&shard.count, -1)
		h.metricsConnGauge.Dec()
		close(c.SendQueue)
	}
}

func (h *Hub) Broadcast(payload []byte) {
	select {
	case h.broadcastQueue <- payload:
		if h.metrics != nil {
			h.metrics.Messages.MessagesPublished.Inc()
		}
	default:
		// queue full, drop message to preserve latency
		if h.metrics != nil {
			h.metrics.Messages.BroadcastDropped.Inc()
		}
	}
}

// ClientCount returns the total number of tracked connections.
func (h *Hub) ClientCount() int {
	var total int32
	for idx := range h.shards {
		total += atomic.LoadInt32(&h.shards[idx].count)
	}
	return int(total)
}

func (h *Hub) pickShard(id uint64) *shard {
	return &h.shards[int(id)%len(h.shards)]
}

func (h *Hub) Shutdown(ctx context.Context) {
	for idx := range h.shards {
		shard := &h.shards[idx]
		shard.clients.Range(func(_, value any) bool {
			conn := value.(*Connection)
			h.Unregister(conn)
			return true
		})
	}

	close(h.broadcastQueue)
}

func (h *Hub) broadcastWorker() {
	for payload := range h.broadcastQueue {
		h.broadcastToShards(payload)
	}
}

func (h *Hub) broadcastToShards(payload []byte) {
	for idx := range h.shards {
		shard := &h.shards[idx]
		shard.clients.Range(func(_, value any) bool {
			conn := value.(*Connection)
			select {
			case conn.SendQueue <- payload:
				if h.metrics != nil {
					h.metrics.Messages.MessagesDelivered.Inc()
				}
			default:
				// channel full; skip to avoid blocking
			}
			return true
		})
	}
}
