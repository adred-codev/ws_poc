package multi

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/koding/websocketproxy"
	"github.com/rs/zerolog"
)

// LoadBalancer distributes incoming WebSocket connections to available shards.
// It uses a "least connections" strategy to ensure even distribution.
type LoadBalancer struct {
	addr    string
	shards  []*Shard
	proxies []http.Handler // One WebSocket proxy per shard
	logger  zerolog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// LoadBalancerConfig holds configuration for the LoadBalancer
type LoadBalancerConfig struct {
	Addr   string
	Shards []*Shard
	Logger zerolog.Logger
}

// NewLoadBalancer creates a new LoadBalancer instance.
func NewLoadBalancer(cfg LoadBalancerConfig) (*LoadBalancer, error) {
	if len(cfg.Shards) == 0 {
		return nil, fmt.Errorf("no shards provided to load balancer")
	}

	ctx, cancel := context.WithCancel(context.Background())

	proxies := make([]http.Handler, len(cfg.Shards))
	for i, shard := range cfg.Shards {
		// Parse shard URL - use ws:// scheme for WebSocket proxy
		shardURL, err := url.Parse(fmt.Sprintf("ws://%s", shard.GetAddr()))
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to parse shard address %s: %w", shard.GetAddr(), err)
		}
		// Create WebSocket-aware proxy using koding/websocketproxy
		// This library properly handles WebSocket upgrade protocol
		proxies[i] = websocketproxy.NewProxy(shardURL)
	}

	lb := &LoadBalancer{
		addr:    cfg.Addr,
		shards:  cfg.Shards,
		proxies: proxies,
		logger:  cfg.Logger.With().Str("component", "load_balancer").Logger(),
		ctx:     ctx,
		cancel:  cancel,
	}

	return lb, nil
}

// Start begins the LoadBalancer's HTTP server.
func (lb *LoadBalancer) Start() error {
	lb.logger.Info().Str("address", lb.addr).Msg("LoadBalancer starting")

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", lb.handleWebSocket)
	mux.HandleFunc("/health", lb.handleHealth)

	server := &http.Server{
		Addr:    lb.addr,
		Handler: mux,
		// Use shorter timeouts for load balancer to quickly reject bad connections
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		IdleTimeout:    10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	lb.wg.Add(1)
	go func() {
		defer lb.wg.Done()
		lb.logger.Info().Str("address", server.Addr).Msg("LoadBalancer listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			lb.logger.Error().Err(err).Msg("LoadBalancer HTTP server error")
		}
	}()

	lb.logger.Info().Msg("LoadBalancer started")
	return nil
}

// Shutdown gracefully stops the LoadBalancer.
func (lb *LoadBalancer) Shutdown() {
	lb.logger.Info().Msg("Shutting down LoadBalancer")
	lb.cancel()
	lb.wg.Wait() // Wait for the Start goroutine to finish
	lb.logger.Info().Msg("LoadBalancer shut down")
}

// handleWebSocket handles incoming WebSocket upgrade requests.
// It selects a shard with available capacity and reserves a slot before proxying.
func (lb *LoadBalancer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Select shard and try to acquire a slot atomically
	selectedShardIndex, selectedShard := lb.selectAndAcquireShard()
	if selectedShard == nil {
		lb.logger.Warn().Msg("No available shards to accept connection")
		http.Error(w, "Server overloaded", http.StatusServiceUnavailable)
		return
	}

	lb.logger.Info().
		Int("shard_id", selectedShard.ID).
		Int("available_slots", selectedShard.GetAvailableSlots()).
		Msg("Proxying connection to shard")

	// Wrap ResponseWriter to release slot when connection closes
	wrappedWriter := &slotReleasingWriter{
		ResponseWriter: w,
		shard:          selectedShard,
		logger:         lb.logger,
		hijacked:       false,
	}

	// Ensure slot is released if hijack fails (e.g., bad handshake)
	defer func() {
		if !wrappedWriter.hijacked {
			selectedShard.ReleaseSlot()
			lb.logger.Debug().
				Int("shard_id", selectedShard.ID).
				Int("available_slots", selectedShard.GetAvailableSlots()).
				Msg("Released slot after failed handshake")
		}
	}()

	// Proxy the WebSocket connection
	lb.proxies[selectedShardIndex].ServeHTTP(wrappedWriter, r)
}

// selectAndAcquireShard atomically selects a shard and acquires a connection slot.
// Returns nil if all shards are at capacity.
// Uses "most available slots first" strategy for better distribution.
func (lb *LoadBalancer) selectAndAcquireShard() (int, *Shard) {
	var (
		mostAvailableSlots int   = -1
		selectedShard      *Shard
		selectedIndex      int = -1
	)

	// First pass: find shard with most available slots
	for i, shard := range lb.shards {
		availableSlots := shard.GetAvailableSlots()

		if availableSlots > mostAvailableSlots {
			mostAvailableSlots = availableSlots
			selectedShard = shard
			selectedIndex = i
		}
	}

	// If no shard has available slots, return nil
	if selectedShard == nil || mostAvailableSlots == 0 {
		return -1, nil
	}

	// Try to acquire slot atomically
	if !selectedShard.TryAcquireSlot() {
		// Race condition: slot was taken between check and acquire
		// Return nil to indicate failure
		return -1, nil
	}

	return selectedIndex, selectedShard
}

// slotReleasingWriter wraps http.ResponseWriter to release shard slot on connection close
type slotReleasingWriter struct {
	http.ResponseWriter
	shard    *Shard
	logger   zerolog.Logger
	hijacked bool
}

// Hijack implements http.Hijacker interface to detect connection close
func (w *slotReleasingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("ResponseWriter does not support hijacking")
	}

	conn, rw, err := hijacker.Hijack()
	if err != nil {
		// If hijack fails, don't mark as hijacked - let defer release the slot
		return nil, nil, err
	}

	// Mark as hijacked so defer doesn't release the slot
	w.hijacked = true

	// Wrap connection to release slot on close
	wrappedConn := &slotReleasingConn{
		Conn:   conn,
		shard:  w.shard,
		logger: w.logger,
	}

	return wrappedConn, rw, nil
}

// slotReleasingConn wraps net.Conn to release shard slot when connection closes
type slotReleasingConn struct {
	net.Conn
	shard  *Shard
	logger zerolog.Logger
	once   sync.Once
}

// Close releases the shard slot and closes the underlying connection
func (c *slotReleasingConn) Close() error {
	c.once.Do(func() {
		c.shard.ReleaseSlot()
		c.logger.Debug().
			Int("shard_id", c.shard.ID).
			Int("available_slots", c.shard.GetAvailableSlots()).
			Msg("Released connection slot")
	})
	return c.Conn.Close()
}

// selectShard selects a shard using the "least connections" strategy.
// It also respects the WS_MAX_CONNECTIONS limit per shard.
func (lb *LoadBalancer) selectShard() (int, *Shard) {
	var (
		leastConnections int64 = math.MaxInt64
		selectedShard    *Shard
		selectedIndex    int = -1
	)

	for i, shard := range lb.shards {
		currentConns := shard.GetCurrentConnections()
		maxConns := int64(shard.GetMaxConnections())

		// Skip shards that are at or over capacity
		if currentConns >= maxConns {
			continue
		}

		// Use < to select first shard with fewest connections
		// This reduces bias toward higher-indexed shards
		if currentConns < leastConnections {
			leastConnections = currentConns
			selectedShard = shard
			selectedIndex = i
		}
	}

	return selectedIndex, selectedShard
}

// handleHealth aggregates health status from all shards.
// Returns a simplified health response matching the expected format.
func (lb *LoadBalancer) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	// Handle preflight requests
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Aggregate metrics from all shards
	var totalConnections int64
	var totalMaxConnections int64
	allShardsHealthy := true

	for _, shard := range lb.shards {
		currentConns := shard.GetCurrentConnections()
		maxConns := int64(shard.GetMaxConnections())

		totalConnections += currentConns
		totalMaxConnections += maxConns

		// Simple health check: if shard is at over capacity, mark as unhealthy
		if currentConns > maxConns {
			allShardsHealthy = false
		}
	}

	// Calculate capacity percentage
	var capacityPercent float64
	if totalMaxConnections > 0 {
		capacityPercent = float64(totalConnections) / float64(totalMaxConnections) * 100
	}

	// Build simplified health response matching expected format
	isHealthy := allShardsHealthy && totalConnections <= totalMaxConnections
	status := "healthy"
	statusCode := http.StatusOK

	if !isHealthy {
		status = "unhealthy"
		statusCode = http.StatusServiceUnavailable
	} else if capacityPercent > 90 {
		status = "degraded"
	}

	response := map[string]interface{}{
		"status":  status,
		"healthy": isHealthy,
		"checks": map[string]interface{}{
			"capacity": map[string]interface{}{
				"current": int(totalConnections),
			},
			"cpu": map[string]interface{}{
				"percentage": 0.0, // Load balancer doesn't track CPU
			},
			"memory": map[string]interface{}{
				"percentage": 0.0, // Load balancer doesn't track memory
			},
		},
	}

	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}
