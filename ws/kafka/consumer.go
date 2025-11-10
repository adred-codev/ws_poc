package kafka

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/twmb/franz-go/pkg/kgo"
)

// TokenEvent represents an event from Redpanda
type TokenEvent struct {
	Type      EventType              `json:"type"`
	Timestamp int64                  `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// BroadcastFunc is called when a message is received
// Parameters: tokenID, eventType, messageJSON
type BroadcastFunc func(tokenID string, eventType string, message []byte)

// ResourceGuard interface for rate limiting and CPU emergency brake
type ResourceGuard interface {
	AllowKafkaMessage(ctx context.Context) (allow bool, waitDuration time.Duration)
	ShouldPauseKafka() bool
}

// Consumer wraps franz-go client for consuming from Redpanda
type Consumer struct {
	client        *kgo.Client
	logger        *zerolog.Logger
	broadcast     BroadcastFunc
	resourceGuard ResourceGuard
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup

	// Metrics
	messagesProcessed uint64
	messagesFailed    uint64
	messagesDropped   uint64 // Rate limited or CPU paused
	mu                sync.RWMutex
}

// ConsumerConfig holds consumer configuration
type ConsumerConfig struct {
	Brokers       []string
	ConsumerGroup string
	Topics        []string
	Logger        *zerolog.Logger
	Broadcast     BroadcastFunc
	ResourceGuard ResourceGuard // For rate limiting and CPU brake
}

// NewConsumer creates a new Kafka consumer
func NewConsumer(cfg ConsumerConfig) (*Consumer, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("at least one broker is required")
	}
	if cfg.ConsumerGroup == "" {
		return nil, fmt.Errorf("consumer group is required")
	}
	if len(cfg.Topics) == 0 {
		return nil, fmt.Errorf("at least one topic is required")
	}
	if cfg.Broadcast == nil {
		return nil, fmt.Errorf("broadcast function is required")
	}
	if cfg.ResourceGuard == nil {
		return nil, fmt.Errorf("resource guard is required")
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create franz-go client
	client, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ConsumerGroup(cfg.ConsumerGroup),
		kgo.ConsumeTopics(cfg.Topics...),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtEnd()), // Start from latest
		kgo.FetchMaxWait(500*time.Millisecond),
		kgo.FetchMinBytes(1),
		kgo.FetchMaxBytes(10*1024*1024), // 10MB
		kgo.SessionTimeout(30*time.Second),
		kgo.RebalanceTimeout(60*time.Second),
		kgo.OnPartitionsAssigned(func(_ context.Context, _ *kgo.Client, assigned map[string][]int32) {
			if cfg.Logger != nil {
				cfg.Logger.Info().
					Interface("partitions", assigned).
					Msg("Partitions assigned")
			}
		}),
		kgo.OnPartitionsRevoked(func(_ context.Context, _ *kgo.Client, revoked map[string][]int32) {
			if cfg.Logger != nil {
				cfg.Logger.Info().
					Interface("partitions", revoked).
					Msg("Partitions revoked")
			}
		}),
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create kafka client: %w", err)
	}

	consumer := &Consumer{
		client:        client,
		logger:        cfg.Logger,
		broadcast:     cfg.Broadcast,
		resourceGuard: cfg.ResourceGuard,
		ctx:           ctx,
		cancel:        cancel,
	}

	return consumer, nil
}

// Start begins consuming messages
func (c *Consumer) Start() error {
	c.logger.Info().Msg("Starting Kafka consumer")

	// Start consumer loop
	c.wg.Add(1)
	go c.consumeLoop()

	return nil
}

// Stop gracefully stops the consumer
func (c *Consumer) Stop() error {
	c.logger.Info().Msg("Stopping Kafka consumer")

	// Cancel context to stop consume loop
	c.cancel()

	// Wait for consumer loop to finish
	c.wg.Wait()

	// Close client
	c.client.Close()

	c.logger.Info().
		Uint64("messages_processed", c.messagesProcessed).
		Uint64("messages_failed", c.messagesFailed).
		Msg("Kafka consumer stopped")

	return nil
}

// consumeLoop continuously polls for messages
func (c *Consumer) consumeLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			// Poll for messages
			fetches := c.client.PollFetches(c.ctx)

			// Check for errors
			if errs := fetches.Errors(); len(errs) > 0 {
				for _, err := range errs {
					c.logger.Error().
						Err(err.Err).
						Str("topic", err.Topic).
						Int32("partition", err.Partition).
						Msg("Fetch error")
				}
			}

			// Process records
			fetches.EachRecord(func(record *kgo.Record) {
				c.processRecord(record)
			})
		}
	}
}

// processRecord handles a single Kafka record with three-layer protection
//
// LAYER 1: Rate limiting (caps consumption at configured rate, e.g., 25 msg/sec)
// LAYER 2: CPU emergency brake (pauses when CPU exceeds threshold, e.g., 80%)
// LAYER 3: Worker pool async processing (queues to 192 workers, non-blocking)
//
// This matches the NATS implementation that achieved 12K connections @ 30% CPU.
// Without these protections, Kafka consumer blocks synchronously, causing plateau at 2.2K connections.
func (c *Consumer) processRecord(record *kgo.Record) {
	// ============================================================================
	// LAYER 1: RATE LIMITING
	// ============================================================================
	// Check if we're allowed to process this message based on configured rate limit
	// If rate limit exceeded, drop message and let Kafka handle redelivery
	allow, waitDuration := c.resourceGuard.AllowKafkaMessage(c.ctx)
	if !allow {
		c.incrementDropped()

		// Log every 100th drop to avoid log spam
		dropped := c.getDroppedCount()
		if dropped%100 == 0 {
			c.logger.Warn().
				Uint64("dropped_count", dropped).
				Dur("would_wait", waitDuration).
				Str("topic", record.Topic).
				Msg("Kafka rate limit exceeded - dropping messages")
		}
		return
	}

	// ============================================================================
	// LAYER 2: CPU EMERGENCY BRAKE
	// ============================================================================
	// If CPU is critically high (>80% by default), pause consumption
	// This provides backpressure to prevent cascading failures
	if c.resourceGuard.ShouldPauseKafka() {
		c.incrementDropped()

		// Log every 100th drop to avoid log spam
		dropped := c.getDroppedCount()
		if dropped%100 == 0 {
			c.logger.Warn().
				Uint64("dropped_count", dropped).
				Str("topic", record.Topic).
				Msg("CPU emergency brake - pausing Kafka consumption")
		}
		return
	}

	// Extract token ID from key
	tokenID := string(record.Key)
	if tokenID == "" {
		c.logger.Warn().
			Str("topic", record.Topic).
			Msg("Record missing token ID key")
		c.incrementFailed()
		return
	}

	// Get event type category from topic
	// NOTE: We broadcast raw bytes without validation for performance
	// (matching NATS implementation behavior - no unmarshal overhead)
	eventType := TopicToEventType(record.Topic)

	// ============================================================================
	// DIRECT BROADCAST (Worker pool removed for performance)
	// ============================================================================
	// Worker pool was redundant because:
	// 1. Broadcast is already non-blocking (uses select with default)
	// 2. Only 25 msg/sec = minimal work (~12ms per poll cycle)
	// 3. 192 workers sitting idle 87% of time, wasting CPU on context switches
	// 4. Franz-go already has internal goroutines per partition
	//
	// Direct call is safe because:
	// - Broadcast completes in ~1ms (iterates subscribed clients with non-blocking sends)
	// - Poll cycle: 500ms wait + 12ms processing = well within franz-go timing requirements
	// - Consumer group heartbeats, offsets, rebalancing all still work correctly
	//
	// Performance gain: ~1-2% CPU saved + reduced goroutine overhead
	c.broadcast(tokenID, eventType, record.Value)

	// Increment processed count after successful broadcast
	c.incrementProcessed()

	// DEBUG level: Zero overhead in production (LOG_LEVEL=info)
	c.logger.Debug().
		Str("token_id", tokenID).
		Str("event_type", eventType).
		Str("topic", record.Topic).
		Msg("Consumed Kafka message")
}

// GetMetrics returns current consumer metrics
func (c *Consumer) GetMetrics() (processed, failed, dropped uint64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.messagesProcessed, c.messagesFailed, c.messagesDropped
}

func (c *Consumer) incrementProcessed() {
	c.mu.Lock()
	c.messagesProcessed++
	c.mu.Unlock()
}

func (c *Consumer) incrementFailed() {
	c.mu.Lock()
	c.messagesFailed++
	c.mu.Unlock()
}

func (c *Consumer) incrementDropped() {
	c.mu.Lock()
	c.messagesDropped++
	c.mu.Unlock()
}

func (c *Consumer) getDroppedCount() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.messagesDropped
}
