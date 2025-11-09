package kafka

import (
	"context"
	"encoding/json"
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

// Consumer wraps franz-go client for consuming from Redpanda
type Consumer struct {
	client    *kgo.Client
	logger    *zerolog.Logger
	broadcast BroadcastFunc
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup

	// Metrics
	messagesProcessed uint64
	messagesFailed    uint64
	mu                sync.RWMutex
}

// ConsumerConfig holds consumer configuration
type ConsumerConfig struct {
	Brokers       []string
	ConsumerGroup string
	Topics        []string
	Logger        *zerolog.Logger
	Broadcast     BroadcastFunc
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
		client:    client,
		logger:    cfg.Logger,
		broadcast: cfg.Broadcast,
		ctx:       ctx,
		cancel:    cancel,
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

// processRecord handles a single Kafka record
func (c *Consumer) processRecord(record *kgo.Record) {
	// Extract token ID from key
	tokenID := string(record.Key)
	if tokenID == "" {
		c.logger.Warn().
			Str("topic", record.Topic).
			Msg("Record missing token ID key")
		c.incrementFailed()
		return
	}

	// Parse event from value
	var event TokenEvent
	if err := json.Unmarshal(record.Value, &event); err != nil {
		c.logger.Error().
			Err(err).
			Str("token_id", tokenID).
			Str("topic", record.Topic).
			Msg("Failed to unmarshal event")
		c.incrementFailed()
		return
	}

	// Get event type category from topic
	eventType := TopicToEventType(record.Topic)

	// Broadcast to clients
	c.broadcast(tokenID, eventType, record.Value)

	// Increment processed count
	c.incrementProcessed()

	// Log every consumed message (visibility like publisher)
	c.logger.Info().
		Str("token_id", tokenID).
		Str("event_type", eventType).
		Str("topic", record.Topic).
		Msg("Consumed Kafka message")
}

// GetMetrics returns current consumer metrics
func (c *Consumer) GetMetrics() (processed, failed uint64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.messagesProcessed, c.messagesFailed
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
