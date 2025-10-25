package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

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

	// Create and configure server with loaded configuration
	serverConfig := ServerConfig{
		Addr:            cfg.Addr,
		NATSUrl:         cfg.NATSUrl,
		MaxConnections:  cfg.MaxConnections,
		BufferSize:      4096, // Constant
		WorkerCount:     cfg.WorkerPoolSize,
		WorkerQueueSize: cfg.WorkerQueueSize,
		NumShards:       cfg.NumShards,

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
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	logger.Println("Shutting down server...")
	if err := server.Shutdown(); err != nil {
		logger.Printf("Error during shutdown: %v", err)
	}
}
