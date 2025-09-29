package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"odin-ws-server/internal/metrics"
	"odin-ws-server/internal/types"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		// Allow connections from any origin in development
		// In production, implement proper origin checking
		return true
	},
	EnableCompression: true,
}

// Client is a middleman between the websocket connection and the hub
type Client struct {
	// The websocket connection
	conn *websocket.Conn

	// Buffered channel of outbound messages
	send chan []byte

	// Client information
	ID          string
	ConnectedAt time.Time

	// Message tracking for deduplication
	seenNonces map[string]time.Time

	// Metrics
	metrics metrics.MetricsInterface

	// Logger
	logger *log.Logger

	// Hub reference for unregistering
	hub *Hub
}

// NewClient creates a new client instance
func NewClient(conn *websocket.Conn, hub *Hub, metrics metrics.MetricsInterface, logger *log.Logger) *Client {
	return &Client{
		conn:        conn,
		send:        make(chan []byte, 256),
		ID:          generateClientID(),
		ConnectedAt: time.Now(),
		seenNonces:  make(map[string]time.Time),
		metrics:     metrics,
		logger:      logger,
		hub:         hub,
	}
}

// readPump pumps messages from the websocket connection to the hub
func (c *Client) readPump() {
	defer func() {
		c.hub.UnregisterClient(c)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.logger.Printf("WebSocket error for client %s: %v", c.ID, err)
				c.metrics.RecordError("websocket_read")
			}
			break
		}

		// Process the received message
		c.handleMessage(message)
		c.metrics.IncrementMessagesReceived()
		c.metrics.RecordMessageSize(len(message))
	}
}

// writePump pumps messages from the hub to the websocket connection
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				c.logger.Printf("Error writing message for client %s: %v", c.ID, err)
				c.metrics.RecordError("websocket_write")
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.logger.Printf("Error sending ping to client %s: %v", c.ID, err)
				c.metrics.RecordError("websocket_ping")
				return
			}
		}
	}
}

// handleMessage processes incoming messages from clients
func (c *Client) handleMessage(message []byte) {
	start := time.Now()
	defer func() {
		c.metrics.RecordMessageLatency(time.Since(start))
	}()

	// Parse the base message to determine type
	var baseMsg types.BaseMessage
	if err := json.Unmarshal(message, &baseMsg); err != nil {
		c.logger.Printf("Error parsing message from client %s: %v", c.ID, err)
		c.metrics.RecordError("message_parse")
		return
	}

	// Check for duplicate messages
	if c.isDuplicateMessage(baseMsg.Nonce) {
		c.metrics.IncrementDuplicates()
		return
	}

	// Handle different message types
	switch baseMsg.Type {
	case types.MessageTypePing:
		c.handlePing(message)
	case types.MessageTypeHeartbeat:
		c.handleHeartbeat()
	default:
		c.logger.Printf("Unknown message type from client %s: %s", c.ID, baseMsg.Type)
		c.metrics.RecordError("unknown_message_type")
	}
}

// handlePing responds to ping messages with pong
func (c *Client) handlePing(message []byte) {
	var pingMsg types.PingMessage
	if err := json.Unmarshal(message, &pingMsg); err != nil {
		c.logger.Printf("Error parsing ping message from client %s: %v", c.ID, err)
		c.metrics.RecordError("ping_parse")
		return
	}

	// Create pong response
	pongMsg := types.PongMessage{
		BaseMessage: types.BaseMessage{
			Type:      types.MessageTypePong,
			Timestamp: time.Now().UnixMilli(),
			Nonce:     generateNonce(),
		},
		OriginalTimestamp: pingMsg.Timestamp,
		ClientID:          c.ID,
	}

	if data, err := json.Marshal(pongMsg); err == nil {
		select {
		case c.send <- data:
		default:
			// Send channel is full, client is too slow
			c.logger.Printf("Client %s send channel full, dropping pong", c.ID)
			c.metrics.RecordError("send_channel_full")
		}
	}
}

// handleHeartbeat responds to heartbeat requests
func (c *Client) handleHeartbeat() {
	heartbeatMsg := types.HeartbeatMessage{
		BaseMessage: types.BaseMessage{
			Type:      types.MessageTypeHeartbeat,
			Timestamp: time.Now().UnixMilli(),
			Nonce:     generateNonce(),
		},
		ServerTime: time.Now().UnixMilli(),
		Uptime:     c.metrics.GetUptime().Milliseconds(),
	}

	if data, err := json.Marshal(heartbeatMsg); err == nil {
		select {
		case c.send <- data:
		default:
			c.logger.Printf("Client %s send channel full, dropping heartbeat", c.ID)
			c.metrics.RecordError("send_channel_full")
		}
	}
}

// isDuplicateMessage checks if we've seen this nonce before for this client
func (c *Client) isDuplicateMessage(nonce string) bool {
	if nonce == "" {
		return false
	}

	// Check if we've seen this nonce before
	if _, exists := c.seenNonces[nonce]; exists {
		return true
	}

	// Store the nonce with current timestamp
	c.seenNonces[nonce] = time.Now()

	// Clean up old nonces (keep only last 100)
	if len(c.seenNonces) > 100 {
		// Remove oldest nonces
		oldest := time.Now()
		oldestNonce := ""
		for n, t := range c.seenNonces {
			if t.Before(oldest) {
				oldest = t
				oldestNonce = n
			}
		}
		if oldestNonce != "" {
			delete(c.seenNonces, oldestNonce)
		}
	}

	return false
}

// ServeWS handles websocket requests from the peer
func ServeWS(hub *Hub, metrics metrics.MetricsInterface, logger *log.Logger, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Printf("WebSocket upgrade error: %v", err)
		metrics.RecordError("websocket_upgrade")
		return
	}

	client := NewClient(conn, hub, metrics, logger)

	// Register the client with the hub
	hub.RegisterClient(client)

	// Start goroutines for reading and writing
	go client.writePump()
	go client.readPump()
}

// Utility functions
func generateClientID() string {
	return "client-" + generateNonce()
}

func generateNonce() string {
	return time.Now().Format("20060102150405.000000") + "-" + randomString(8)
}

func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}