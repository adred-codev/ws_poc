package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"odin-ws-server/internal/metrics"
	"odin-ws-server/internal/types"
)

type Client struct {
	conn      *nats.Conn
	metrics   metrics.MetricsInterface
	subs      map[string]*nats.Subscription
	subsMutex sync.RWMutex
	handlers  map[string]func([]byte)
	logger    *log.Logger
}

type Config struct {
	URL               string
	MaxReconnects     int
	ReconnectWait     time.Duration
	ReconnectJitter   time.Duration
	MaxPingsOut       int
	PingInterval      time.Duration
}

func NewClient(config Config, metrics metrics.MetricsInterface, logger *log.Logger) (*Client, error) {
	opts := []nats.Option{
		nats.MaxReconnects(config.MaxReconnects),
		nats.ReconnectWait(config.ReconnectWait),
		nats.ReconnectJitter(config.ReconnectJitter, config.ReconnectJitter),
		nats.MaxPingsOutstanding(config.MaxPingsOut),
		nats.PingInterval(config.PingInterval),
	}

	client := &Client{
		metrics:  metrics,
		subs:     make(map[string]*nats.Subscription),
		handlers: make(map[string]func([]byte)),
		logger:   logger,
	}

	// Add connection event handlers
	opts = append(opts,
		nats.ConnectHandler(client.connectHandler),
		nats.DisconnectErrHandler(client.disconnectHandler),
		nats.ReconnectHandler(client.reconnectHandler),
		nats.ErrorHandler(client.errorHandler),
	)

	conn, err := nats.Connect(config.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	client.conn = conn
	client.metrics.SetNATSConnected(true)

	return client, nil
}

// Connection event handlers
func (c *Client) connectHandler(conn *nats.Conn) {
	c.logger.Printf("Connected to NATS at %s", conn.ConnectedUrl())
	c.metrics.SetNATSConnected(true)
}

func (c *Client) disconnectHandler(conn *nats.Conn, err error) {
	if err != nil {
		c.logger.Printf("Disconnected from NATS with error: %v", err)
		c.metrics.RecordError("nats_disconnect")
	} else {
		c.logger.Printf("Disconnected from NATS")
	}
	c.metrics.SetNATSConnected(false)
}

func (c *Client) reconnectHandler(conn *nats.Conn) {
	c.logger.Printf("Reconnected to NATS at %s", conn.ConnectedUrl())
	c.metrics.SetNATSConnected(true)
	c.metrics.IncrementNATSReconnects()
}

func (c *Client) errorHandler(conn *nats.Conn, sub *nats.Subscription, err error) {
	c.logger.Printf("NATS error: %v", err)
	c.metrics.RecordError("nats_error")
}

// Subscribe to a subject with a handler function
func (c *Client) Subscribe(subject string, handler func([]byte)) error {
	c.subsMutex.Lock()
	defer c.subsMutex.Unlock()

	// Store the handler
	c.handlers[subject] = handler

	// Create subscription
	sub, err := c.conn.Subscribe(subject, func(msg *nats.Msg) {
		start := time.Now()

		// Call the handler
		handler(msg.Data)

		// Record metrics
		c.metrics.IncrementNATSMessages()
		c.metrics.RecordNATSLatency(time.Since(start))
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", subject, err)
	}

	c.subs[subject] = sub
	c.logger.Printf("Subscribed to NATS subject: %s", subject)

	return nil
}

// Unsubscribe from a subject
func (c *Client) Unsubscribe(subject string) error {
	c.subsMutex.Lock()
	defer c.subsMutex.Unlock()

	sub, exists := c.subs[subject]
	if !exists {
		return fmt.Errorf("not subscribed to subject: %s", subject)
	}

	if err := sub.Unsubscribe(); err != nil {
		return fmt.Errorf("failed to unsubscribe from %s: %w", subject, err)
	}

	delete(c.subs, subject)
	delete(c.handlers, subject)
	c.logger.Printf("Unsubscribed from NATS subject: %s", subject)

	return nil
}

// Publish a message to a subject
func (c *Client) Publish(subject string, data []byte) error {
	start := time.Now()

	if err := c.conn.Publish(subject, data); err != nil {
		c.metrics.RecordError("nats_publish")
		return fmt.Errorf("failed to publish to %s: %w", subject, err)
	}

	c.metrics.RecordNATSLatency(time.Since(start))
	return nil
}

// PublishJSON publishes a JSON-serializable object
func (c *Client) PublishJSON(subject string, obj interface{}) error {
	data, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return c.Publish(subject, data)
}

// Request-reply pattern
func (c *Client) Request(subject string, data []byte, timeout time.Duration) (*nats.Msg, error) {
	start := time.Now()

	msg, err := c.conn.Request(subject, data, timeout)
	if err != nil {
		c.metrics.RecordError("nats_request")
		return nil, fmt.Errorf("failed to send request to %s: %w", subject, err)
	}

	c.metrics.RecordNATSLatency(time.Since(start))
	return msg, nil
}

// Health check
func (c *Client) IsConnected() bool {
	return c.conn != nil && c.conn.IsConnected()
}

func (c *Client) Status() nats.Status {
	if c.conn == nil {
		return nats.DISCONNECTED
	}
	return c.conn.Status()
}

func (c *Client) Stats() nats.Statistics {
	if c.conn == nil {
		return nats.Statistics{}
	}
	return c.conn.Stats()
}

// Graceful shutdown
func (c *Client) Close() error {
	c.subsMutex.Lock()
	defer c.subsMutex.Unlock()

	// Unsubscribe from all subjects
	for subject, sub := range c.subs {
		if err := sub.Unsubscribe(); err != nil {
			c.logger.Printf("Error unsubscribing from %s: %v", subject, err)
		}
	}

	// Close connection
	if c.conn != nil {
		c.conn.Close()
		c.metrics.SetNATSConnected(false)
		c.logger.Printf("NATS connection closed")
	}

	return nil
}

// Wait for connection to be established
func (c *Client) WaitForConnection(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if c.IsConnected() {
				return nil
			}
		}
	}
}

// Subject builders for Odin message types
type Subjects struct{}

func (s Subjects) TokenPrice(tokenID string) string {
	return fmt.Sprintf("odin.token.%s.price", tokenID)
}

func (s Subjects) TokenVolume(tokenID string) string {
	return fmt.Sprintf("odin.token.%s.volume", tokenID)
}

func (s Subjects) BatchUpdate() string {
	return "odin.token.batch.update"
}

func (s Subjects) Trades(tokenID string) string {
	return fmt.Sprintf("odin.trades.%s", tokenID)
}

func (s Subjects) MarketStats() string {
	return "odin.market.statistics"
}

func (s Subjects) Heartbeat() string {
	return "odin.heartbeat"
}

// Global subjects instance
var SubjectBuilder = Subjects{}

// Utility function to parse NATS message into Odin message types
func ParseMessage(data []byte) (types.MessageType, interface{}, error) {
	// First, determine the message type
	var base types.BaseMessage
	if err := json.Unmarshal(data, &base); err != nil {
		return "", nil, fmt.Errorf("failed to parse base message: %w", err)
	}

	// Parse into specific message type
	switch base.Type {
	case types.MessageTypePriceUpdate:
		var msg types.PriceUpdateMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return "", nil, fmt.Errorf("failed to parse price update message: %w", err)
		}
		return base.Type, msg, nil

	case types.MessageTypeTradeExecuted:
		var msg types.TradeExecutedMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return "", nil, fmt.Errorf("failed to parse trade executed message: %w", err)
		}
		return base.Type, msg, nil

	case types.MessageTypeVolumeUpdate:
		var msg types.VolumeUpdateMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return "", nil, fmt.Errorf("failed to parse volume update message: %w", err)
		}
		return base.Type, msg, nil

	case types.MessageTypeBatchUpdate:
		var msg types.BatchUpdateMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return "", nil, fmt.Errorf("failed to parse batch update message: %w", err)
		}
		return base.Type, msg, nil

	case types.MessageTypeMarketStats:
		var msg types.MarketStatsMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return "", nil, fmt.Errorf("failed to parse market stats message: %w", err)
		}
		return base.Type, msg, nil

	case types.MessageTypeHeartbeat:
		var msg types.HeartbeatMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return "", nil, fmt.Errorf("failed to parse heartbeat message: %w", err)
		}
		return base.Type, msg, nil

	default:
		return base.Type, base, nil
	}
}