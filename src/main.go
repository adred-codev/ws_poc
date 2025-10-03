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
		addr  = flag.String("addr", ":3002", "server address")
		nats  = flag.String("nats", "", "NATS server URL (optional)")
		debug = flag.Bool("debug", false, "enable debug logging")
	)
	flag.Parse()

	// Create logger
	logger := log.New(os.Stdout, "[WS] ", log.LstdFlags)
	if !*debug {
		logger.SetOutput(os.Stdout)
	}

	// automaxprocs automatically sets GOMAXPROCS based on container CPU limits
	// Get the detected CPU count for worker pool sizing
	maxProcs := runtime.GOMAXPROCS(0)
	logger.Printf("Detected CPU limit: %d cores (via automaxprocs)", maxProcs)

	// Detect container memory limit and calculate safe max connections
	memLimit, err := getMemoryLimit()
	if err != nil {
		logger.Printf("Warning: Could not detect memory limit: %v (using defaults)", err)
	}
	maxConnections := calculateMaxConnections(memLimit)

	if memLimit > 0 {
		logger.Printf("Detected memory limit: %d MB (%.2f GB)", memLimit/(1024*1024), float64(memLimit)/(1024*1024*1024))
		logger.Printf("Calculated max connections: %d (based on 180KB per connection + 128MB overhead)", maxConnections)
	} else {
		logger.Printf("No memory limit detected, using default: %d connections", maxConnections)
	}

	// Create and configure server
	config := ServerConfig{
		Addr:           *addr,
		NATSUrl:        *nats,
		MaxConnections: maxConnections,
		BufferSize:     4096,
		WorkerCount:    maxProcs * 2, // 2 workers per CPU core
	}

	server, err := NewServer(config, logger)
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
