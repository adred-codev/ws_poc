package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/adred-codev/ws_poc/internal/multi"
	"github.com/adred-codev/ws_poc/internal/shared/monitoring"
	"github.com/adred-codev/ws_poc/internal/shared/platform"
	"github.com/adred-codev/ws_poc/internal/shared/types"
	_ "go.uber.org/automaxprocs"
)

// Helper function to split broker string
func splitBrokers(brokers string) []string {
	result := []string{}
	for _, b := range strings.Split(brokers, ",") {
		trimmed := strings.TrimSpace(b)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func main() {
	var (
		debug      = flag.Bool("debug", false, "enable debug logging (overrides LOG_LEVEL)")
		numShards  = flag.Int("shards", 3, "number of shards to run")
		basePort   = flag.Int("base-port", 3002, "base port for shards (e.g., 3002, 3003, ...)")
		lbAddr     = flag.String("lb-addr", ":3005", "address for the load balancer to listen on")
	)
	flag.Parse()

	// Create basic logger for startup
	logger := log.New(os.Stdout, "[WS-MULTI] ", log.LstdFlags)

	// automaxprocs automatically sets GOMAXPROCS based on container CPU limits
	maxProcs := runtime.GOMAXPROCS(0)
	logger.Printf("GOMAXPROCS: %d (via automaxprocs - rounds down to integer)", maxProcs)

	// Load configuration from .env file and environment variables
	cfg, err := platform.LoadConfig(nil) // Pass nil for now, structured logger created after
	if err != nil {
		logger.Fatalf("Failed to load configuration: %v", err)
	}

	// Override debug mode if flag set
	if *debug {
		cfg.LogLevel = "debug"
		logger.Printf("Debug mode enabled via flag")
	}

	// Print human-readable config for startup logs
	cfg.Print()

	// Create and configure server with loaded configuration
	kafkaBrokers := []string{}
	if cfg.KafkaBrokers != "" {
		kafkaBrokers = splitBrokers(cfg.KafkaBrokers)
	}

	// Calculate max connections per shard
	maxConnsPerShard := cfg.MaxConnections / *numShards
	if maxConnsPerShard == 0 {
		maxConnsPerShard = 1 // Ensure at least 1 connection per shard
	}
	logger.Printf("Total Max Connections: %d, Shards: %d, Max Connections per Shard: %d", cfg.MaxConnections, *numShards, maxConnsPerShard)

	// Initialize central BroadcastBus
	broadcastBus := multi.NewBroadcastBus(1024, monitoring.NewLogger(monitoring.LoggerConfig{
		Level:  types.LogLevel(cfg.LogLevel),
		Format: types.LogFormat(cfg.LogFormat),
	}))
	broadcastBus.Run()

	// Create and start shards
	shards := make([]*multi.Shard, *numShards)
	for i := 0; i < *numShards; i++ {
		// Bind address: 0.0.0.0 to accept both IPv4/IPv6 connections
		shardBindAddr := fmt.Sprintf("0.0.0.0:%d", *basePort+i)
		// Advertise address: 127.0.0.1 (IPv4 loopback) for LoadBalancer to connect to
		shardAdvertiseAddr := fmt.Sprintf("127.0.0.1:%d", *basePort+i)

		shardConfig := types.ServerConfig{
			Addr:           shardBindAddr,
			KafkaBrokers:   kafkaBrokers,
			ConsumerGroup:  cfg.ConsumerGroup, // Base consumer group name
			MaxConnections: maxConnsPerShard,  // Shard-specific max connections

			CPULimit:               cfg.CPULimit,
			MemoryLimit:            cfg.MemoryLimit,
			MaxKafkaMessagesPerSec: cfg.MaxKafkaRate,
			MaxBroadcastsPerSec:    cfg.MaxBroadcastRate,
			MaxGoroutines:          cfg.MaxGoroutines,
			CPURejectThreshold:     cfg.CPURejectThreshold,
			CPUPauseThreshold:      cfg.CPUPauseThreshold,
			MetricsInterval:        cfg.MetricsInterval,
			LogLevel:               types.LogLevel(cfg.LogLevel),
			LogFormat:              types.LogFormat(cfg.LogFormat),
		}

		shard, err := multi.NewShard(multi.ShardConfig{
			ID:             i,
			Addr:           shardBindAddr,      // Bind address for listening
			AdvertiseAddr:  shardAdvertiseAddr, // Address for LoadBalancer connections
			ServerConfig:   shardConfig,
			BroadcastBus:   broadcastBus, // Pass reference to bus, shard will subscribe internally
			Logger:         monitoring.NewLogger(monitoring.LoggerConfig{Level: types.LogLevel(cfg.LogLevel), Format: types.LogFormat(cfg.LogFormat)}),
			MaxConnections: maxConnsPerShard,
		})
		if err != nil {
			logger.Fatalf("Failed to create shard %d: %v", i, err)
		}

		if err := shard.Start(); err != nil {
			logger.Fatalf("Failed to start shard %d: %v", i, err)
		}
		shards[i] = shard
	}

	// Initialize and start LoadBalancer
	lb, err := multi.NewLoadBalancer(multi.LoadBalancerConfig{
		Addr:   *lbAddr,
		Shards: shards,
		Logger: monitoring.NewLogger(monitoring.LoggerConfig{Level: types.LogLevel(cfg.LogLevel), Format: types.LogFormat(cfg.LogFormat)}),
	})
	if err != nil {
		logger.Fatalf("Failed to create load balancer: %v", err)
	}
	if err := lb.Start(); err != nil {
		logger.Fatalf("Failed to start load balancer: %v", err)
	}

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	logger.Println("Shutting down multi-core server...")

	// Shutdown LoadBalancer
	lb.Shutdown()

	// Shutdown shards
	for _, shard := range shards {
		if err := shard.Shutdown(); err != nil {
			logger.Printf("Error during shard %d shutdown: %v", shard.ID, err)
		}
	}

	// Shutdown BroadcastBus
	broadcastBus.Shutdown()

	logger.Println("Multi-core server gracefully shut down.")
}
