package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

func main() {
	var (
		addr  = flag.String("addr", ":3002", "server address")
		nats  = flag.String("nats", "", "NATS server URL (optional)")
		debug = flag.Bool("debug", false, "enable debug logging")
	)
	flag.Parse()

	// Set GOMAXPROCS to use all CPUs
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Create logger
	logger := log.New(os.Stdout, "[WS] ", log.LstdFlags)
	if !*debug {
		logger.SetOutput(os.Stdout)
	}

	// Create and configure server
	config := ServerConfig{
		Addr:           *addr,
		NATSUrl:        *nats,
		MaxConnections: 10000,
		BufferSize:     4096,
		WorkerCount:    runtime.NumCPU() * 2,
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
