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

// LRUCache implements a simple LRU cache for nonce deduplication
type LRUCache struct {
	cache map[string]*cacheNode
	head, tail *cacheNode
	capacity int
	mu sync.RWMutex
}

type cacheNode struct {
	key string
	value time.Time
	prev, next *cacheNode
}

func NewLRUCache(capacity int) *LRUCache {
	head := &cacheNode{}
	tail := &cacheNode{}
	head.next = tail
	tail.prev = head

	return &LRUCache{
		cache: make(map[string]*cacheNode, capacity),
		head: head,
		tail: tail,
		capacity: capacity,
	}
}

func (c *LRUCache) Get(key string) (time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if node, exists := c.cache[key]; exists {
		c.moveToHead(node)
		return node.value, true
	}
	return time.Time{}, false
}

func (c *LRUCache) Put(key string, value time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if node, exists := c.cache[key]; exists {
		node.value = value
		c.moveToHead(node)
		return
	}

	if len(c.cache) >= c.capacity {
		tail := c.removeTail()
		delete(c.cache, tail.key)
	}

	newNode := &cacheNode{key: key, value: value}
	c.cache[key] = newNode
	c.addToHead(newNode)
}

func (c *LRUCache) addToHead(node *cacheNode) {
	node.prev = c.head
	node.next = c.head.next
	c.head.next.prev = node
	c.head.next = node
}

func (c *LRUCache) removeNode(node *cacheNode) {
	node.prev.next = node.next
	node.next.prev = node.prev
}

func (c *LRUCache) moveToHead(node *cacheNode) {
	c.removeNode(node)
	c.addToHead(node)
}

func (c *LRUCache) removeTail() *cacheNode {
	tail := c.tail.prev
	c.removeNode(tail)
	return tail
}

// Hub maintains the set of active clients and broadcasts messages to them
type Hub struct {
	// Registered clients - use slice for better cache locality
	clients []*Client
	clientsMutex sync.RWMutex

	// Inbound messages from clients
	broadcast chan []byte

	// Register requests from clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	// Simplified nonce tracking with LRU cache
	nonces *LRUCache

	// Metrics (simplified)
	metrics metrics.MetricsInterface

	// Logger
	logger *log.Logger

	// Graceful shutdown
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Pre-allocated message buffers
	messagePool sync.Pool
}

func NewHub(metricsInstance metrics.MetricsInterface, logger *log.Logger) *Hub {
	ctx, cancel := context.WithCancel(context.Background())

	hub := &Hub{
		// Balanced optimization for stable 5K connections
		clients:   make([]*Client, 0, 5000),  // Realistic pre-allocation
		broadcast: make(chan []byte, 8192),   // Good buffer without excess
		register:  make(chan *Client, 1000),  // Reasonable burst handling
		unregister: make(chan *Client, 1000),
		nonces:    NewLRUCache(50000), // Practical nonce cache size
		metrics:   metricsInstance,
		logger:    logger,
		ctx:       ctx,
		cancel:    cancel,
	}

	// Larger message pool buffers with available memory
	hub.messagePool.New = func() interface{} {
		return make([]byte, 0, 1024) // Bigger buffers with 380MB RAM headroom
	}

	return hub
}


func (h *Hub) Run() {
	h.wg.Add(1)
	defer h.wg.Done()

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
	h.clientsMutex.Lock()
	h.clients = append(h.clients, client)
	clientCount := len(h.clients)
	h.clientsMutex.Unlock()

	h.metrics.IncrementConnections()
	h.logger.Printf("Client %s connected. Total clients: %d", client.ID, clientCount)

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
	h.clientsMutex.Lock()
	// Find and remove client from slice
	for i, c := range h.clients {
		if c == client {
			// Remove by swapping with last element
			h.clients[i] = h.clients[len(h.clients)-1]
			h.clients = h.clients[:len(h.clients)-1]
			break
		}
	}
	clientCount := len(h.clients)
	h.clientsMutex.Unlock()

	close(client.send)
	h.metrics.DecrementConnections()
	h.metrics.RecordConnectionDuration(time.Since(client.ConnectedAt))
	h.logger.Printf("Client %s disconnected. Total clients: %d", client.ID, clientCount)
}

func (h *Hub) forceUnregisterClient(client *Client) {
	h.clientsMutex.Lock()
	// Find and remove client from slice
	for i, c := range h.clients {
		if c == client {
			// Remove by swapping with last element
			h.clients[i] = h.clients[len(h.clients)-1]
			h.clients = h.clients[:len(h.clients)-1]
			break
		}
	}
	clientCount := len(h.clients)
	h.clientsMutex.Unlock()

	close(client.send)
	client.conn.Close()
	h.metrics.DecrementConnections()
	h.metrics.RecordConnectionError()
	h.logger.Printf("Client %s force disconnected. Total clients: %d", client.ID, clientCount)
}

func (h *Hub) broadcastMessage(message []byte) {
	// Check for message deduplication using LRU cache
	if h.isDuplicateMessage(message) {
		h.metrics.IncrementDuplicates()
		return
	}

	h.metrics.IncrementMessagesSent()
	h.metrics.RecordMessageSize(len(message))

	var clientsToUnregister []*Client

	// Use read lock for better concurrency
	h.clientsMutex.RLock()
	for _, client := range h.clients {
		select {
		case client.send <- message:
			// Message sent successfully
		default:
			// Client's send channel is full, mark for removal
			clientsToUnregister = append(clientsToUnregister, client)
		}
	}
	h.clientsMutex.RUnlock()

	// Unregister clients whose send channels were full
	for _, client := range clientsToUnregister {
		h.forceUnregisterClient(client)
	}
}

func (h *Hub) isDuplicateMessage(message []byte) bool {
	// Fast path: extract nonce without full JSON parsing
	nonce := h.extractNonce(message)
	if nonce == "" {
		return false
	}

	// Check LRU cache for duplicate
	if _, exists := h.nonces.Get(nonce); exists {
		return true
	}

	// Store nonce in LRU cache
	h.nonces.Put(nonce, time.Now())
	return false
}

// extractNonce quickly extracts nonce from JSON without full parsing
func (h *Hub) extractNonce(message []byte) string {
	// Simple string search for nonce field - much faster than JSON parsing
	nonceStart := []byte(`"nonce":"`)
	idx := indexOf(message, nonceStart)
	if idx == -1 {
		return ""
	}

	start := idx + len(nonceStart)
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
	h.clientsMutex.RLock()
	count := len(h.clients)
	h.clientsMutex.RUnlock()
	return count
}

// GetStats returns hub statistics
func (h *Hub) GetStats() map[string]interface{} {
	h.clientsMutex.RLock()
	clientCount := len(h.clients)
	h.clientsMutex.RUnlock()

	return map[string]interface{}{
		"connected_clients": clientCount,
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
	h.clientsMutex.RLock()
	for _, client := range h.clients {
		client.conn.Close()
	}
	h.clientsMutex.RUnlock()

	// Wait for hub goroutine to finish
	h.wg.Wait()
	h.logger.Println("Hub shutdown complete")
}