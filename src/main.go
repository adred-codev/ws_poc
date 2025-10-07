package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	_ "go.uber.org/automaxprocs"
)

// getEnvFloat reads a float64 environment variable with a default value
func getEnvFloat(key string, defaultValue float64) float64 {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.ParseFloat(val, 64); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// getEnvInt reads an int environment variable with a default value
func getEnvInt(key string, defaultValue int) int {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// getEnvInt64 reads an int64 environment variable with a default value
func getEnvInt64(key string, defaultValue int64) int64 {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.ParseInt(val, 10, 64); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// getEnvDuration reads a duration environment variable with a default value
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if parsed, err := time.ParseDuration(val); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// getEnvString reads a string environment variable with a default value
func getEnvString(key string, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

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

	// Create and configure server with environment variable overrides
	config := ServerConfig{
		Addr:           *addr,
		NATSUrl:        *nats,
		MaxConnections: maxConnections,
		BufferSize:     4096,
		WorkerCount:    maxProcs * 2, // 2 workers per CPU core

		// Dynamic capacity thresholds (overridable via env vars)
		CPURejectThreshold: getEnvFloat("CPU_REJECT_THRESHOLD", 80.0),
		CPUPauseThreshold:  getEnvFloat("CPU_PAUSE_THRESHOLD", 85.0),
		CPUTargetMax:       getEnvFloat("CPU_TARGET_MAX", 70.0),

		// Capacity limits
		MinConnections: getEnvInt("MIN_CONNECTIONS", 50),
		MaxCapacity:    getEnvInt("MAX_CAPACITY", 10000),
		SafetyMargin:   getEnvFloat("SAFETY_MARGIN", 0.9),

		// JetStream configuration
		JSStreamMaxAge:    getEnvDuration("JS_STREAM_MAX_AGE", 30*time.Second),
		JSStreamMaxMsgs:   getEnvInt64("JS_STREAM_MAX_MSGS", 100000),
		JSStreamMaxBytes:  getEnvInt64("JS_STREAM_MAX_BYTES", 50*1024*1024), // 50MB
		JSConsumerAckWait: getEnvDuration("JS_CONSUMER_ACK_WAIT", 30*time.Second),
		JSStreamName:      getEnvString("JS_STREAM_NAME", "ODIN_TOKENS"),
		JSConsumerName:    getEnvString("JS_CONSUMER_NAME", "ws-server"),

		// Monitoring intervals
		MetricsInterval:  getEnvDuration("METRICS_INTERVAL", 15*time.Second),
		CapacityInterval: getEnvDuration("CAPACITY_INTERVAL", 30*time.Second),
	}

	logger.Printf("Configuration:")
	logger.Printf("  CPU Reject Threshold: %.1f%%", config.CPURejectThreshold)
	logger.Printf("  CPU Pause Threshold: %.1f%%", config.CPUPauseThreshold)
	logger.Printf("  CPU Target Max: %.1f%%", config.CPUTargetMax)
	logger.Printf("  Capacity Range: %d - %d connections", config.MinConnections, config.MaxCapacity)
	logger.Printf("  JetStream Stream: %s (max age: %s)", config.JSStreamName, config.JSStreamMaxAge)
	logger.Printf("  Monitoring: metrics=%s, capacity=%s", config.MetricsInterval, config.CapacityInterval)

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
