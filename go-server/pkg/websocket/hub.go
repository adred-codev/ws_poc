package websocket

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"odin-ws-server/internal/metrics"
	"odin-ws-server/internal/types"
)

// Hub maintains the set of active clients and broadcasts messages to them
type Hub struct {
	// Registered clients
	clients map[*Client]bool

	// Inbound messages from clients
	broadcast chan []byte

	// Register requests from clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	// Message tracking for deduplication
	seenNonces map[string]time.Time
	nonceMutex sync.RWMutex

	// Metrics
	metrics *metrics.Metrics

	// Enhanced metrics for detailed tracking
	enhancedMetrics *metrics.EnhancedMetrics

	// Message rate tracking
	messageRateTracker *metrics.MessageRateTracker

	// Logger
	logger *log.Logger

	// Graceful shutdown
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewHub(metricsInstance *metrics.Metrics, logger *log.Logger) *Hub {
	ctx, cancel := context.WithCancel(context.Background())

	return &Hub{
		clients:            make(map[*Client]bool),
		broadcast:          make(chan []byte, 1000), // Buffered channel for high throughput
		register:           make(chan *Client, 100),
		unregister:         make(chan *Client, 100),
		seenNonces:         make(map[string]time.Time),
		metrics:            metricsInstance,
		enhancedMetrics:    nil, // Will be set later by the server
		messageRateTracker: metrics.NewMessageRateTracker(),
		logger:             logger,
		ctx:                ctx,
		cancel:             cancel,
	}
}

// SetEnhancedMetrics sets the enhanced metrics instance
func (h *Hub) SetEnhancedMetrics(enhancedMetrics *metrics.EnhancedMetrics) {
	h.enhancedMetrics = enhancedMetrics
}

func (h *Hub) Run() {
	h.wg.Add(1)
	defer h.wg.Done()

	// Start cleanup routine for old nonces
	go h.cleanupNonces()

	// Start message rate calculation
	go h.updateMessageRate()

	for {
		select {
		case <-h.ctx.Done():
			h.logger.Println("Hub shutting down...")
			return

		case client := <-h.register:
			h.registerClient(client)

		case client := <-h.unregister:
			h.unregisterClient(client)

		case message := <-h.broadcast:
			h.broadcastMessage(message)
		}
	}
}

func (h *Hub) registerClient(client *Client) {
	h.clients[client] = true
	h.metrics.IncrementConnections()

	// Also track in enhanced metrics if available
	if h.enhancedMetrics != nil {
		remoteAddr := "unknown"
		if client.conn != nil {
			remoteAddr = client.conn.RemoteAddr().String()
		}
		h.enhancedMetrics.AddConnection(client.ID, remoteAddr)
	}

	h.logger.Printf("Client %s connected. Total clients: %d", client.ID, len(h.clients))

	// Send connection established message
	connMsg := types.ConnectionEstablishedMessage{
		BaseMessage: types.BaseMessage{
			Type:      types.MessageTypeConnectionEstablished,
			Timestamp: time.Now().UnixMilli(),
			Nonce:     generateNonce(),
		},
		ClientID:     client.ID,
		ServerTime:   time.Now().UnixMilli(),
		Capabilities: []string{"price-updates", "trade-notifications", "market-stats"},
	}

	if msgData, err := json.Marshal(connMsg); err == nil {
		select {
		case client.send <- msgData:
		default:
			// Client's send channel is full, close it
			h.forceUnregisterClient(client)
		}
	}
}

func (h *Hub) unregisterClient(client *Client) {
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.send)
		h.metrics.DecrementConnections()
		h.metrics.RecordConnectionDuration(time.Since(client.ConnectedAt))

		// Also remove from enhanced metrics if available
		if h.enhancedMetrics != nil {
			h.enhancedMetrics.RemoveConnection(client.ID)
		}

		h.logger.Printf("Client %s disconnected. Total clients: %d", client.ID, len(h.clients))
	}
}

func (h *Hub) forceUnregisterClient(client *Client) {
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.send)
		client.conn.Close()
		h.metrics.DecrementConnections()
		h.metrics.RecordConnectionError()

		h.logger.Printf("Client %s force disconnected. Total clients: %d", client.ID, len(h.clients))
	}
}

func (h *Hub) broadcastMessage(message []byte) {
	// Check for message deduplication
	if h.isDuplicateMessage(message) {
		h.metrics.IncrementDuplicates()
		return
	}

	h.metrics.IncrementMessagesSent()
	h.metrics.RecordMessageSize(len(message))

	// Broadcast to all connected clients
	var wg sync.WaitGroup
	for client := range h.clients {
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()

			select {
			case c.send <- message:
				// Message sent successfully
			default:
				// Client's send channel is full, remove client
				h.forceUnregisterClient(c)
			}
		}(client)
	}

	// Don't wait for all goroutines to complete to maintain high throughput
	// The goroutines will complete in the background
}

func (h *Hub) isDuplicateMessage(message []byte) bool {
	// Parse message to extract nonce
	var baseMsg types.BaseMessage
	if err := json.Unmarshal(message, &baseMsg); err != nil {
		return false // If we can't parse it, assume it's not a duplicate
	}

	if baseMsg.Nonce == "" {
		return false // No nonce means we can't deduplicate
	}

	h.nonceMutex.Lock()
	defer h.nonceMutex.Unlock()

	// Check if we've seen this nonce before
	if _, exists := h.seenNonces[baseMsg.Nonce]; exists {
		return true
	}

	// Store the nonce with current timestamp
	h.seenNonces[baseMsg.Nonce] = time.Now()
	return false
}

func (h *Hub) cleanupNonces() {
	ticker := time.NewTicker(5 * time.Minute) // Cleanup every 5 minutes
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			h.performNonceCleanup()
		}
	}
}

func (h *Hub) performNonceCleanup() {
	h.nonceMutex.Lock()
	defer h.nonceMutex.Unlock()

	cutoff := time.Now().Add(-10 * time.Minute) // Remove nonces older than 10 minutes
	for nonce, timestamp := range h.seenNonces {
		if timestamp.Before(cutoff) {
			delete(h.seenNonces, nonce)
		}
	}

	h.logger.Printf("Cleaned up old nonces. Current nonce count: %d", len(h.seenNonces))
}

func (h *Hub) updateMessageRate() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			// Get current message count and update rate
			// This is a simplified rate calculation
			h.messageRateTracker.Update(float64(h.metrics.GetActiveConnections()))
			rate := h.messageRateTracker.GetRate()
			h.metrics.UpdateMessagesPerSecond(rate)
		}
	}
}

// RegisterClient adds a client to the hub
func (h *Hub) RegisterClient(client *Client) {
	select {
	case h.register <- client:
	case <-h.ctx.Done():
		h.logger.Println("Hub is shutting down, cannot register client")
	}
}

// UnregisterClient removes a client from the hub
func (h *Hub) UnregisterClient(client *Client) {
	select {
	case h.unregister <- client:
	case <-h.ctx.Done():
		// Hub is shutting down, client will be cleaned up
	}
}

// BroadcastMessage sends a message to all connected clients
func (h *Hub) BroadcastMessage(message []byte) {
	select {
	case h.broadcast <- message:
	case <-h.ctx.Done():
		h.logger.Println("Hub is shutting down, cannot broadcast message")
	default:
		// Broadcast channel is full, drop the message
		h.logger.Println("Broadcast channel full, dropping message")
		h.metrics.RecordError("broadcast_dropped")
	}
}

// GetClientCount returns the number of connected clients
func (h *Hub) GetClientCount() int {
	return len(h.clients)
}

// GetStats returns hub statistics
func (h *Hub) GetStats() map[string]interface{} {
	h.nonceMutex.RLock()
	nonceCount := len(h.seenNonces)
	h.nonceMutex.RUnlock()

	return map[string]interface{}{
		"connected_clients": len(h.clients),
		"seen_nonces":      nonceCount,
		"broadcast_queue":  len(h.broadcast),
		"register_queue":   len(h.register),
		"unregister_queue": len(h.unregister),
	}
}

// Shutdown gracefully shuts down the hub
func (h *Hub) Shutdown() {
	h.logger.Println("Shutting down hub...")
	h.cancel()

	// Close all client connections
	for client := range h.clients {
		client.conn.Close()
	}

	// Wait for hub goroutine to finish
	h.wg.Wait()
	h.logger.Println("Hub shutdown complete")
}