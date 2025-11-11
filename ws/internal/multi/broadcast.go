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

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewBroadcastBus creates a new BroadcastBus instance.
// bufferSize determines the capacity of the internal publish channel.
func NewBroadcastBus(bufferSize int, logger zerolog.Logger) *BroadcastBus {
	ctx, cancel := context.WithCancel(context.Background())
	return &BroadcastBus{
		publishCh:   make(chan *BroadcastMessage, bufferSize),
		subscribers: make([]chan *BroadcastMessage, 0),
		logger:      logger.With().Str("component", "broadcast_bus").Logger(),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Run starts the BroadcastBus's main loop, fanning out messages to subscribers.
func (b *BroadcastBus) Run() {
	b.logger.Info().Msg("BroadcastBus started")
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		for {
			select {
			case msg := <-b.publishCh:
				b.fanOut(msg)
			case <-b.ctx.Done():
				b.logger.Info().Msg("BroadcastBus stopped")
				return
			}
		}
	}()
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

	for _, subCh := range b.subscribers {
		select {
		case subCh <- msg:
			// Message sent to subscriber
		case <-b.ctx.Done():
			return // Bus is shutting down
		default:
			// This indicates a slow subscriber, message dropped for this shard
			b.logger.Warn().Msg("Subscriber channel full, message dropped for a shard")
		}
	}
}
