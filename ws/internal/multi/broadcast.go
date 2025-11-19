package multi

import (
	"context"
	"sync"

	"github.com/rs/zerolog"
)

// BroadcastBus is a central in-memory pub/sub system for inter-shard communication.
// It receives messages from Kafka consumers (via shards) and fans them out to all subscribing shards.
type BroadcastBus struct {
	publishCh   chan *BroadcastMessage          // Shards publish messages here
	subscribers []chan *BroadcastMessage        // Each shard subscribes to one of these
	mu          sync.RWMutex                    // Protects access to subscribers
	logger      zerolog.Logger                  // Logger for the bus

	// Batching configuration
	batchSize    int // Max messages to drain per batch (default: 100, 0 = disabled)
	batchEnabled bool

	// Metrics
	batchesSent   uint64
	messagesSent  uint64
	messagesDropped uint64
	metricsMu     sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewBroadcastBus creates a new BroadcastBus instance.
// bufferSize determines the capacity of the internal publish channel.
func NewBroadcastBus(bufferSize int, logger zerolog.Logger) *BroadcastBus {
	ctx, cancel := context.WithCancel(context.Background())

	// Enable batching by default for performance
	batchSize := 100 // Drain up to 100 messages per batch
	batchEnabled := batchSize > 1

	busLogger := logger.With().Str("component", "broadcast_bus").Logger()
	if batchEnabled {
		busLogger.Info().
			Int("batch_size", batchSize).
			Msg("Broadcast message batching enabled")
	}

	return &BroadcastBus{
		publishCh:    make(chan *BroadcastMessage, bufferSize),
		subscribers:  make([]chan *BroadcastMessage, 0),
		logger:       busLogger,
		batchSize:    batchSize,
		batchEnabled: batchEnabled,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Run starts the BroadcastBus's main loop, fanning out messages to subscribers.
func (b *BroadcastBus) Run() {
	b.logger.Info().Msg("BroadcastBus started")
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()

		if !b.batchEnabled {
			// Unbatched mode: process one message at a time
			b.runUnbatched()
			return
		}

		// Batched mode: drain multiple messages and fan out in batch
		batch := make([]*BroadcastMessage, 0, b.batchSize)
		for {
			select {
			case msg := <-b.publishCh:
				batch = append(batch, msg)

				// Drain additional messages from channel (up to batchSize)
			drainLoop:
				for len(batch) < b.batchSize {
					select {
					case msg := <-b.publishCh:
						batch = append(batch, msg)
					default:
						// No more messages available, break drain loop
						break drainLoop
					}
				}

				// Fan out batch
				b.fanOutBatch(batch)

				// Clear batch for reuse
				batch = batch[:0]

			case <-b.ctx.Done():
				// Fan out any remaining messages before shutdown
				if len(batch) > 0 {
					b.fanOutBatch(batch)
				}
				b.logger.Info().Msg("BroadcastBus stopped")
				return
			}
		}
	}()
}

// runUnbatched is the original implementation without batching
func (b *BroadcastBus) runUnbatched() {
	for {
		select {
		case msg := <-b.publishCh:
			b.fanOut(msg)
		case <-b.ctx.Done():
			b.logger.Info().Msg("BroadcastBus stopped")
			return
		}
	}
}

// Shutdown gracefully stops the BroadcastBus.
func (b *BroadcastBus) Shutdown() {
	b.logger.Info().Msg("Shutting down BroadcastBus")
	b.cancel()
	b.wg.Wait() // Wait for the Run goroutine to finish

	// Close all subscriber channels
	b.mu.Lock()
	for _, subCh := range b.subscribers {
		close(subCh)
	}
	b.subscribers = nil
	b.mu.Unlock()
}

// Publish sends a message to the BroadcastBus.
// This is called by shards when they receive a message from Kafka.
func (b *BroadcastBus) Publish(msg *BroadcastMessage) {
	select {
	case b.publishCh <- msg:
		// Message sent to bus
	case <-b.ctx.Done():
		b.logger.Warn().Msg("BroadcastBus is shutting down, message not published")
	default:
		// This should ideally not happen if bufferSize is adequate
		b.logger.Warn().Msg("BroadcastBus publish channel is full, message dropped")
	}
}

// Subscribe returns a channel that a shard can listen on for broadcast messages.
func (b *BroadcastBus) Subscribe() chan *BroadcastMessage {
	subCh := make(chan *BroadcastMessage, cap(b.publishCh)) // Buffer same as publish channel
	b.mu.Lock()
	b.subscribers = append(b.subscribers, subCh)
	b.mu.Unlock()
	return subCh
}

// fanOut sends a message to all subscribed shards.
func (b *BroadcastBus) fanOut(msg *BroadcastMessage) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	sent := 0
	for _, subCh := range b.subscribers {
		select {
		case subCh <- msg:
			sent++
		case <-b.ctx.Done():
			return // Bus is shutting down
		default:
			// This indicates a slow subscriber, message dropped for this shard
			b.logger.Warn().Msg("Subscriber channel full, message dropped for a shard")
			b.incrementDropped()
		}
	}

	if sent > 0 {
		b.incrementSent(1)
	}
}

// fanOutBatch sends multiple messages to all subscribed shards efficiently.
func (b *BroadcastBus) fanOutBatch(batch []*BroadcastMessage) {
	if len(batch) == 0 {
		return
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	totalSent := 0
	totalDropped := 0

	// Send each message to all subscribers
	for _, msg := range batch {
		for _, subCh := range b.subscribers {
			select {
			case subCh <- msg:
				totalSent++
			case <-b.ctx.Done():
				return // Bus is shutting down
			default:
				// Subscriber channel full, drop message
				totalDropped++
			}
		}
	}

	// Update metrics
	if totalSent > 0 {
		b.incrementSent(uint64(len(batch)))
	}
	if totalDropped > 0 {
		b.incrementDropped()
		if totalDropped%100 == 0 {
			b.logger.Warn().
				Int("dropped", totalDropped).
				Int("batch_size", len(batch)).
				Msg("Dropped messages due to slow subscribers")
		}
	}

	// Track batching metrics
	b.incrementBatches()

	// Log batching metrics periodically (only when debug enabled)
	// IMPORTANT: Guard with level check to avoid expensive mutex locks in getBatchCount()/getMessageCount()
	// Cost when disabled: ~1ns (level check) vs ~150ns (modulo + 2 mutex locks)
	if b.logger.GetLevel() <= zerolog.DebugLevel {
		if batches := b.getBatchCount(); batches%100 == 0 {
			b.logger.Debug().
				Uint64("batches_sent", batches).
				Uint64("messages_sent", b.getMessageCount()).
				Int("last_batch_size", len(batch)).
				Msg("Broadcast batching metrics")
		}
	}
}

// Metrics helper methods
func (b *BroadcastBus) incrementSent(count uint64) {
	b.metricsMu.Lock()
	b.messagesSent += count
	b.metricsMu.Unlock()
}

func (b *BroadcastBus) incrementDropped() {
	b.metricsMu.Lock()
	b.messagesDropped++
	b.metricsMu.Unlock()
}

func (b *BroadcastBus) incrementBatches() {
	b.metricsMu.Lock()
	b.batchesSent++
	b.metricsMu.Unlock()
}

func (b *BroadcastBus) getBatchCount() uint64 {
	b.metricsMu.RLock()
	defer b.metricsMu.RUnlock()
	return b.batchesSent
}

func (b *BroadcastBus) getMessageCount() uint64 {
	b.metricsMu.RLock()
	defer b.metricsMu.RUnlock()
	return b.messagesSent
}
