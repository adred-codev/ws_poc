package sharded

import (
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// MessageRouter distributes messages to shards that have subscribers
// CRITICAL OPTIMIZATION: Only sends to shards with active subscribers,
// not all shards. This reduces CPU usage from O(all_shards) to O(subscribed_shards)
type MessageRouter struct {
	shards    []*Shard
	numShards int

	// Optimization: Track which shards have subscribers for each channel
	// This map is updated infrequently (only on subscribe/unsubscribe)
	// Read frequently (on every broadcast)
	// RWMutex allows many concurrent reads with occasional writes
	channelShards map[string]map[int]bool // channel → set of shard IDs
	mu            sync.RWMutex

	// Metrics
	messagesRouted int64
	routingTime    int64 // Total nanoseconds spent routing

	logger *log.Logger
}

// NewMessageRouter creates a new message router with the specified number of shards
func NewMessageRouter(numShards int, logger *log.Logger) *MessageRouter {
	if numShards <= 0 {
		numShards = runtime.NumCPU() * 2 // Default: 2 shards per CPU core
	}

	router := &MessageRouter{
		shards:        make([]*Shard, numShards),
		numShards:     numShards,
		channelShards: make(map[string]map[int]bool),
		logger:        logger,
	}

	// Create shards with CPU affinity
	for i := 0; i < numShards; i++ {
		cpuCore := i % runtime.NumCPU()
		router.shards[i] = NewShard(i, cpuCore, logger)
		go router.shards[i].Run()
	}

	logger.Printf("[MessageRouter] Created %d shards across %d CPU cores",
		numShards, runtime.NumCPU())

	return router
}

// Route sends a message to all shards that have subscribers for this channel
// CRITICAL: This is called from NATS callback - must be fast!
// Performance target: < 100µs per broadcast
func (r *MessageRouter) Route(channel string, data []byte) {
	start := time.Now()

	// Get timestamp ONCE for all shards (saves syscalls)
	timestamp := time.Now().UnixMilli()

	// Read which shards have subscribers (RWMutex read lock - allows concurrent reads)
	r.mu.RLock()
	shardSet, exists := r.channelShards[channel]
	r.mu.RUnlock()

	if !exists || len(shardSet) == 0 {
		return // No subscribers for this channel
	}

	// Create broadcast message
	msg := BroadcastMsg{
		Channel:   channel,
		Data:      data,
		Timestamp: timestamp,
	}

	// Send to ONLY shards with subscribers
	// Example: If only 3 out of 8 shards have subscribers, we only send to those 3
	// This saves 62% of broadcast work!
	for shardID := range shardSet {
		// Non-blocking send to prevent slow shard from blocking router
		select {
		case r.shards[shardID].broadcast <- msg:
			// Success
		default:
			// Shard's broadcast channel full (should be rare with 2048 buffer)
			r.logger.Printf("[MessageRouter] WARNING: Shard %d broadcast channel full", shardID)
		}
	}

	// Update metrics
	atomic.AddInt64(&r.messagesRouted, 1)
	atomic.AddInt64(&r.routingTime, time.Since(start).Nanoseconds())
}

// AssignClient determines which shard a client should be assigned to
// Uses consistent hashing based on client ID for even distribution
func (r *MessageRouter) AssignClient(clientID int64) int {
	return int(clientID) % r.numShards
}

// RegisterClient registers a client with its assigned shard
func (r *MessageRouter) RegisterClient(client *Client) {
	shardID := r.AssignClient(client.ID)
	client.ShardID = shardID
	r.shards[shardID].register <- client
}

// UnregisterClient removes a client from its assigned shard
func (r *MessageRouter) UnregisterClient(client *Client) {
	if client.ShardID < 0 || client.ShardID >= r.numShards {
		r.logger.Printf("[MessageRouter] Invalid shard ID %d for client %d",
			client.ShardID, client.ID)
		return
	}
	r.shards[client.ShardID].unregister <- client
}

// Subscribe subscribes a client to a channel
// Updates both the shard's subscription list AND the router's channel→shard mapping
func (r *MessageRouter) Subscribe(client *Client, channel string) {
	if client.ShardID < 0 || client.ShardID >= r.numShards {
		r.logger.Printf("[MessageRouter] Invalid shard ID %d for client %d",
			client.ShardID, client.ID)
		return
	}

	shard := r.shards[client.ShardID]

	// Tell shard to add subscription
	shard.subscribe <- SubscribeCmd{
		Client:  client,
		Channel: channel,
	}

	// Update router's channel→shard mapping
	// This is the CRITICAL OPTIMIZATION: We track which shards have subscribers
	r.mu.Lock()
	if r.channelShards[channel] == nil {
		r.channelShards[channel] = make(map[int]bool)
	}
	r.channelShards[channel][client.ShardID] = true
	r.mu.Unlock()

	r.logger.Printf("[MessageRouter] Client %d (shard %d) subscribed to %s",
		client.ID, client.ShardID, channel)
}

// Unsubscribe unsubscribes a client from a channel
func (r *MessageRouter) Unsubscribe(client *Client, channel string) {
	if client.ShardID < 0 || client.ShardID >= r.numShards {
		return
	}

	shard := r.shards[client.ShardID]

	// Tell shard to remove subscription
	shard.unsubscribe <- UnsubscribeCmd{
		Client:  client,
		Channel: channel,
	}

	// Note: We don't remove from channelShards immediately because:
	// 1. Shard might still have other clients subscribed to this channel
	// 2. It's safe to send to shard even if it has no subscribers (shard will ignore)
	// 3. Cleanup happens periodically in CleanupRoutingTable()

	r.logger.Printf("[MessageRouter] Client %d (shard %d) unsubscribed from %s",
		client.ID, client.ShardID, channel)
}

// CleanupRoutingTable removes stale entries from channel→shard mapping
// Should be called periodically (e.g., every 60 seconds)
// This prevents memory leak from channels that no longer have any subscribers
func (r *MessageRouter) CleanupRoutingTable() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for channel, shardSet := range r.channelShards {
		// Check each shard to see if it actually has subscribers
		activeShards := make(map[int]bool)

		for shardID := range shardSet {
			shard := r.shards[shardID]
			stats := shard.GetStats()

			// If shard has subscribers for this channel, keep it
			if count, exists := stats.SubscriberCount[channel]; exists && count > 0 {
				activeShards[shardID] = true
			}
		}

		// Update or remove channel entry
		if len(activeShards) == 0 {
			delete(r.channelShards, channel)
		} else {
			r.channelShards[channel] = activeShards
		}
	}

	r.logger.Printf("[MessageRouter] Cleaned up routing table (%d channels tracked)",
		len(r.channelShards))
}

// GetStats returns aggregate statistics across all shards
func (r *MessageRouter) GetStats() ServerStats {
	var totalClients int64
	var totalMessages int64
	var totalDropped int64

	shardStats := make([]ShardStats, r.numShards)

	for i, shard := range r.shards {
		stats := shard.GetStats()
		shardStats[i] = stats

		totalClients += int64(stats.ClientCount)
		totalMessages += stats.MessagesSent
		totalDropped += stats.MessagesDropped
	}

	return ServerStats{
		TotalClients:   totalClients,
		TotalMessages:  totalMessages,
		TotalDropped:   totalDropped,
		ShardsActive:   r.numShards,
		GoroutineCount: runtime.NumGoroutine(),
		ShardStats:     shardStats,
	}
}

// GetAverageRoutingTime returns average time spent routing each message (in microseconds)
func (r *MessageRouter) GetAverageRoutingTime() float64 {
	routed := atomic.LoadInt64(&r.messagesRouted)
	if routed == 0 {
		return 0
	}

	totalNanos := atomic.LoadInt64(&r.routingTime)
	return float64(totalNanos) / float64(routed) / 1000.0 // Convert to microseconds
}

// Shutdown gracefully shuts down all shards
func (r *MessageRouter) Shutdown() {
	r.logger.Printf("[MessageRouter] Shutting down all shards")

	for _, shard := range r.shards {
		shard.Shutdown()
	}

	// Wait a bit for shards to finish
	time.Sleep(1 * time.Second)

	r.logger.Printf("[MessageRouter] Shutdown complete")
}

// StartCleanupTimer starts a background goroutine that periodically cleans up the routing table
func (r *MessageRouter) StartCleanupTimer() {
	ticker := time.NewTicker(60 * time.Second)
	go func() {
		for range ticker.C {
			r.CleanupRoutingTable()
		}
	}()
}

// GetChannelDistribution returns how many shards have subscribers for each channel
// Useful for debugging and capacity planning
func (r *MessageRouter) GetChannelDistribution() map[string]int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	distribution := make(map[string]int)
	for channel, shards := range r.channelShards {
		distribution[channel] = len(shards)
	}
	return distribution
}

// PrintStats prints detailed statistics about the router
func (r *MessageRouter) PrintStats() {
	stats := r.GetStats()
	r.logger.Printf("=== MessageRouter Statistics ===")
	r.logger.Printf("Total Clients: %d", stats.TotalClients)
	r.logger.Printf("Total Messages: %d", stats.TotalMessages)
	r.logger.Printf("Total Dropped: %d", stats.TotalDropped)
	r.logger.Printf("Active Shards: %d", stats.ShardsActive)
	r.logger.Printf("Goroutines: %d", stats.GoroutineCount)
	r.logger.Printf("Avg Routing Time: %.2f µs", r.GetAverageRoutingTime())

	r.logger.Printf("\n--- Per-Shard Stats ---")
	for _, shardStat := range stats.ShardStats {
		r.logger.Printf("Shard %d: clients=%d, sent=%d, dropped=%d, channels=%d",
			shardStat.ID, shardStat.ClientCount, shardStat.MessagesSent,
			shardStat.MessagesDropped, len(shardStat.SubscriberCount))
	}

	r.logger.Printf("\n--- Channel Distribution ---")
	distribution := r.GetChannelDistribution()
	for channel, shardCount := range distribution {
		r.logger.Printf("Channel %s: %d shards", channel, shardCount)
	}
	r.logger.Printf("================================")
}
