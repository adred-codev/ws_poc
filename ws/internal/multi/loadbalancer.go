package multi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// LoadBalancer distributes incoming WebSocket connections to available shards.
// It uses a "least connections" strategy to ensure even distribution.
type LoadBalancer struct {
	addr    string
	shards  []*Shard
	proxies []*httputil.ReverseProxy // One proxy per shard
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

	proxies := make([]*httputil.ReverseProxy, len(cfg.Shards))
	for i, shard := range cfg.Shards {
		shardURL, err := url.Parse(fmt.Sprintf("http://%s", shard.GetAddr()))
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to parse shard address %s: %w", shard.GetAddr(), err)
		}
		// Use NewSingleHostReverseProxy for Go 1.25+
		proxies[i] = httputil.NewSingleHostReverseProxy(shardURL)
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
	// TODO: Add health check endpoint for the load balancer itself

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
// It selects a shard using the "least connections" strategy and forwards the connection.
func (lb *LoadBalancer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Select shard using "least connections" strategy
	selectedShardIndex, selectedShard := lb.selectShard()
	if selectedShard == nil {
		lb.logger.Warn().Msg("No available shards to accept connection")
		http.Error(w, "Server overloaded", http.StatusServiceUnavailable)
		return
	}

	lb.logger.Info().Int("shard_id", selectedShard.ID).Msg("Proxying connection to shard")
	lb.proxies[selectedShardIndex].ServeHTTP(w, r)
}

// selectShard selects a shard using the "least connections" strategy.
// It also respects the WS_MAX_CONNECTIONS limit per shard.
func (lb *LoadBalancer) selectShard() (int, *Shard) {
	var (
		leastConnections int64 = -1
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

		if selectedShard == nil || currentConns < leastConnections {
			leastConnections = currentConns
			selectedShard = shard
			selectedIndex = i
		}
	}

	return selectedIndex, selectedShard
}
