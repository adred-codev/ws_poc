package main

import (
	"context"
	"flag"
	"go-server-2/sharded"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	_ "go.uber.org/automaxprocs"
)

func main() {
	var (
		debug = flag.Bool("debug", false, "enable debug logging (overrides LOG_LEVEL)")
	)
	flag.Parse()

	// Create basic logger for startup
	logger := log.New(os.Stdout, "[WS] ", log.LstdFlags)

	// automaxprocs automatically sets GOMAXPROCS based on container CPU limits
	// IMPORTANT: automaxprocs rounds DOWN (e.g., 1.5 cores → GOMAXPROCS=1)
	// This is correct for Go scheduler, but we use actual CPU limit for worker sizing
	maxProcs := runtime.GOMAXPROCS(0)
	logger.Printf("GOMAXPROCS: %d (via automaxprocs - rounds down to integer)", maxProcs)

	// Load configuration from .env file and environment variables
	cfg, err := LoadConfig(nil) // Pass nil for now, structured logger created after
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

	// Validate mode
	if cfg.Mode != "monolithic" && cfg.Mode != "sharded" {
		logger.Fatalf("Invalid WS_MODE: %s (must be 'monolithic' or 'sharded')", cfg.Mode)
	}

	// Wait for interrupt signal (setup early)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Start server based on architecture mode
	if cfg.Mode == "sharded" {
		// ===== SHARDED MODE =====
		logger.Printf("Starting in SHARDED mode...")

		// Create sharded server configuration
		shardedConfig := sharded.ShardedServerConfig{
			Addr:           cfg.Addr,
			NATSUrl:        cfg.NATSUrl,
			MaxConnections: cfg.MaxConnections,
			NumShards:      cfg.NumShards, // 0 = auto (2x CPU cores)

			// JetStream configuration (same as monolithic)
			JSStreamName:      cfg.JSStreamName,
			JSConsumerName:    cfg.JSConsumerName,
			JSStreamMaxAge:    cfg.JSStreamMaxAge,
			JSStreamMaxMsgs:   cfg.JSStreamMaxMsgs,
			JSStreamMaxBytes:  cfg.JSStreamMaxBytes,
			JSConsumerAckWait: cfg.JSConsumerAckWait,
		}

		shardedServer, err := sharded.NewShardedServer(shardedConfig)
		if err != nil {
			logger.Fatalf("Failed to create sharded server: %v", err)
		}

		// Start sharded server
		if err := shardedServer.Start(); err != nil {
			logger.Fatalf("Failed to start sharded server: %v", err)
		}

		// Wait for interrupt signal
		<-sigCh
		logger.Println("Shutting down sharded server...")

		// Graceful shutdown with 10 second timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := shardedServer.Shutdown(shutdownCtx); err != nil {
			logger.Printf("Error during sharded server shutdown: %v", err)
		}

	} else {
		// ===== MONOLITHIC MODE =====
		logger.Printf("Starting in MONOLITHIC mode...")

		// Create and configure server with loaded configuration
		serverConfig := ServerConfig{
			Addr:            cfg.Addr,
			NATSUrl:         cfg.NATSUrl,
			MaxConnections:  cfg.MaxConnections,
			BufferSize:      4096, // Constant
			WorkerCount:     cfg.WorkerPoolSize,
			WorkerQueueSize: cfg.WorkerQueueSize,

			// Static resource limits (explicit from config)
			CPULimit:    cfg.CPULimit,
			MemoryLimit: cfg.MemoryLimit,

			// Rate limiting (CRITICAL - prevents overload)
			MaxNATSMessagesPerSec: cfg.MaxNATSRate,
			MaxBroadcastsPerSec:   cfg.MaxBroadcastRate,
			MaxGoroutines:         cfg.MaxGoroutines,

			// Safety thresholds (emergency brakes)
			CPURejectThreshold: cfg.CPURejectThreshold,
			CPUPauseThreshold:  cfg.CPUPauseThreshold,

			// JetStream configuration
			JSStreamMaxAge:    cfg.JSStreamMaxAge,
			JSStreamMaxMsgs:   cfg.JSStreamMaxMsgs,
			JSStreamMaxBytes:  cfg.JSStreamMaxBytes,
			JSConsumerAckWait: cfg.JSConsumerAckWait,
			JSStreamName:      cfg.JSStreamName,
			JSConsumerName:    cfg.JSConsumerName,

			// Monitoring intervals
			MetricsInterval: cfg.MetricsInterval,

			// Logging configuration
			LogLevel:  LogLevel(cfg.LogLevel),
			LogFormat: LogFormat(cfg.LogFormat),
		}

		server, err := NewServer(serverConfig, logger)
		if err != nil {
			logger.Fatalf("Failed to create server: %v", err)
		}

		// Start server
		if err := server.Start(); err != nil {
			logger.Fatalf("Failed to start server: %v", err)
		}

		// Wait for interrupt signal
		<-sigCh
		logger.Println("Shutting down monolithic server...")

		if err := server.Shutdown(); err != nil {
			logger.Printf("Error during monolithic server shutdown: %v", err)
		}
	}
}
