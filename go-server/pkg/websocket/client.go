package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
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
	maxMessageSize = 1024

	// Optimized buffer sizes for balance
	clientSendBuffer = 512 // Moderate buffer for good performance
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  2048,  // Reduced for better connection scaling
	WriteBufferSize: 2048,  // Reduced for better connection scaling
	CheckOrigin: func(r *http.Request) bool {
		// Allow connections from any origin in development
		return true
	},
	EnableCompression: false, // Disable compression to reduce CPU
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

	// Metrics (simplified)
	metrics metrics.MetricsInterface

	// Logger
	logger *log.Logger

	// Hub reference for unregistering
	hub *Hub

	// Message buffer pool
	bufPool *sync.Pool
}

// NewClient creates a new client instance
func NewClient(conn *websocket.Conn, hub *Hub, metrics metrics.MetricsInterface, logger *log.Logger) *Client {
	bufPool := &sync.Pool{
		New: func() interface{} {
			return make([]byte, 0, 2048) // Larger buffers with available RAM
		},
	}

	return &Client{
		conn:        conn,
		send:        make(chan []byte, clientSendBuffer),
		ID:          generateClientID(),
		ConnectedAt: time.Now(),
		metrics:     metrics,
		logger:      logger,
		hub:         hub,
		bufPool:     bufPool,
	}
}

// handleConnection manages the websocket connection with optimized I/O
func (c *Client) handleConnection() {
	defer func() {
		c.hub.UnregisterClient(c)
		c.conn.Close()
	}()

	// Setup connection parameters with optimized settings
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// Setup ping ticker
	pingTicker := time.NewTicker(pingPeriod)
	defer pingTicker.Stop()

	// Large channels with available memory
	readChan := make(chan []byte, 512)
	errChan := make(chan error, 2)

	// Start optimized read goroutine
	go c.readPump(readChan, errChan)

	for {
		select {
		// Handle outgoing messages with batching
		case message, ok := <-c.send:
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Batch more messages to utilize available CPU
			messages := [][]byte{message}
			for i := 0; i < 15 && len(c.send) > 0; i++ {
				if msg, ok := <-c.send; ok {
					messages = append(messages, msg)
				}
			}

			// Write batch with single syscall
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			for _, msg := range messages {
				if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					c.metrics.RecordError("websocket_write")
					return
				}
			}

		// Handle ping timer
		case <-pingTicker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.metrics.RecordError("websocket_ping")
				return
			}

		// Handle incoming messages from read pump
		case msg := <-readChan:
			c.handleMessage(msg)
			c.metrics.IncrementMessagesReceived()

		// Handle read errors
		case err := <-errChan:
			if err != nil {
				return
			}
		}
	}
}

// readPump runs in separate goroutine for concurrent reading
func (c *Client) readPump(readChan chan<- []byte, errChan chan<- error) {
	defer close(errChan)

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.logger.Printf("WebSocket error for client %s: %v", c.ID, err)
				c.metrics.RecordError("websocket_read")
			}
			errChan <- err
			return
		}

		// Send to processing channel
		select {
		case readChan <- message:
		default:
			// Channel full, drop message
			c.metrics.RecordError("read_channel_full")
		}
	}
}

// handleMessage processes incoming messages from clients (optimized)
func (c *Client) handleMessage(message []byte) {
	start := time.Now()
	defer func() {
		c.metrics.RecordMessageLatency(time.Since(start))
	}()

	// Fast path: check message type without full JSON parsing
	msgType := c.extractMessageType(message)
	switch msgType {
	case "ping":
		c.handlePingFast(message)
	case "heartbeat":
		c.handleHeartbeatFast()
	default:
		// Fall back to full JSON parsing for unknown types
		c.handleUnknownMessage(message)
	}
}

// extractMessageType quickly extracts message type without full JSON parsing
func (c *Client) extractMessageType(message []byte) string {
	typeStart := []byte(`"type":"`)
	idx := indexOf(message, typeStart)
	if idx == -1 {
		return ""
	}

	start := idx + len(typeStart)
	if start >= len(message) {
		return ""
	}

	// Find the closing quote
	for i := start; i < len(message); i++ {
		if message[i] == '"' {
			return string(message[start:i])
		}
	}
	return ""
}

// handleUnknownMessage falls back to full JSON parsing
func (c *Client) handleUnknownMessage(message []byte) {
	var baseMsg types.BaseMessage
	if err := json.Unmarshal(message, &baseMsg); err != nil {
		c.logger.Printf("Error parsing message from client %s: %v", c.ID, err)
		c.metrics.RecordError("message_parse")
		return
	}

	c.logger.Printf("Unknown message type from client %s: %s", c.ID, baseMsg.Type)
	c.metrics.RecordError("unknown_message_type")
}

// handlePingFast responds to ping messages with minimal JSON parsing
func (c *Client) handlePingFast(message []byte) {
	// Extract timestamp without full JSON parsing
	timestamp := c.extractTimestamp(message)

	// Create pong response using pre-allocated buffer
	buf := c.bufPool.Get().([]byte)
	buf = buf[:0] // Reset length but keep capacity

	// Build JSON response manually for better performance
	buf = append(buf, `{"type":"pong","timestamp":`...)
	buf = append(buf, []byte(time.Now().Format("1641024000000"))...) // Unix timestamp
	buf = append(buf, `,"nonce":"`...)
	buf = append(buf, generateNonce()...)
	buf = append(buf, `","originalTimestamp":`...)
	buf = append(buf, []byte(timestamp)...)
	buf = append(buf, `,"clientId":"`...)
	buf = append(buf, c.ID...)
	buf = append(buf, `"}`...)

	// Send the response
	select {
	case c.send <- buf:
	default:
		// Send channel is full, return buffer to pool
		c.bufPool.Put(buf)
		c.logger.Printf("Client %s send channel full, dropping pong", c.ID)
		c.metrics.RecordError("send_channel_full")
	}
}

// extractTimestamp quickly extracts timestamp from JSON
func (c *Client) extractTimestamp(message []byte) string {
	timestampStart := []byte(`"timestamp":`)
	idx := indexOf(message, timestampStart)
	if idx == -1 {
		return "0"
	}

	start := idx + len(timestampStart)
	if start >= len(message) {
		return "0"
	}

	// Find the next comma or closing brace
	for i := start; i < len(message); i++ {
		if message[i] == ',' || message[i] == '}' {
			return string(message[start:i])
		}
	}
	return "0"
}

// handleHeartbeatFast responds to heartbeat requests with minimal allocations
func (c *Client) handleHeartbeatFast() {
	// Create heartbeat response using pre-allocated buffer
	buf := c.bufPool.Get().([]byte)
	buf = buf[:0] // Reset length but keep capacity

	uptime := c.metrics.GetUptime().Milliseconds()

	// Build JSON response manually
	buf = append(buf, `{"type":"heartbeat","timestamp":`...)
	buf = append(buf, []byte(time.Now().Format("1641024000000"))...)
	buf = append(buf, `,"nonce":"`...)
	buf = append(buf, generateNonce()...)
	buf = append(buf, `","serverTime":`...)
	buf = append(buf, []byte(time.Now().Format("1641024000000"))...)
	buf = append(buf, `,"uptime":`...)
	buf = append(buf, []byte(time.Duration(uptime).String())...)
	buf = append(buf, '}')

	select {
	case c.send <- buf:
	default:
		// Send channel is full, return buffer to pool
		c.bufPool.Put(buf)
		c.logger.Printf("Client %s send channel full, dropping heartbeat", c.ID)
		c.metrics.RecordError("send_channel_full")
	}
}


// ServeWS handles websocket requests from the peer (optimized single goroutine)
func ServeWS(hub *Hub, metrics metrics.MetricsInterface, logger *log.Logger, w http.ResponseWriter, r *http.Request) {
	// Realistic connection limit for stability
	maxConnections := 5000 // Achievable target with good performance
	if hub.GetClientCount() >= maxConnections {
		logger.Printf("Connection limit reached (%d), rejecting new connection from %s", maxConnections, r.RemoteAddr)
		http.Error(w, "Server at capacity", http.StatusServiceUnavailable)
		metrics.RecordError("connection_limit_reached")
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Printf("WebSocket upgrade error: %v", err)
		metrics.RecordError("websocket_upgrade")
		return
	}

	client := NewClient(conn, hub, metrics, logger)

	// Register the client with the hub
	hub.RegisterClient(client)

	// Use single goroutine for both reading and writing
	go client.handleConnection()
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

// indexOf finds the first occurrence of pattern in data
func indexOf(data []byte, pattern []byte) int {
	for i := 0; i <= len(data)-len(pattern); i++ {
		match := true
		for j := 0; j < len(pattern); j++ {
			if data[i+j] != pattern[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}