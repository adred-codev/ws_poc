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

	// Get resource limits from environment (matching docker-compose)
	cpuLimit := getEnvFloat("WS_CPU_LIMIT", float64(maxProcs))
	memLimit := getEnvInt64("WS_MEMORY_LIMIT", 512*1024*1024) // Default 512MB

	// Get or calculate max connections
	maxConnections := getEnvInt("WS_MAX_CONNECTIONS", 500)

	// Worker pool sizing
	workerCount := getEnvInt("WS_WORKER_POOL_SIZE", maxProcs*2)
	workerQueueSize := getEnvInt("WS_WORKER_QUEUE_SIZE", workerCount*100)

	logger.Printf("Resource Limits:")
	logger.Printf("  CPU Limit: %.1f cores", cpuLimit)
	logger.Printf("  Memory Limit: %d MB", memLimit/(1024*1024))
	logger.Printf("  Max Connections: %d", maxConnections)
	logger.Printf("  Worker Pool: %d workers, %d queue", workerCount, workerQueueSize)

	// Create and configure server with static resource limits
	config := ServerConfig{
		Addr:            *addr,
		NATSUrl:         *nats,
		MaxConnections:  maxConnections,
		BufferSize:      4096,
		WorkerCount:     workerCount,
		WorkerQueueSize: workerQueueSize,

		// Static resource limits (explicit from docker/env)
		CPULimit:    cpuLimit,
		MemoryLimit: memLimit,

		// Rate limiting (CRITICAL - prevents overload)
		MaxNATSMessagesPerSec: getEnvInt("WS_MAX_NATS_RATE", 20),
		MaxBroadcastsPerSec:   getEnvInt("WS_MAX_BROADCAST_RATE", 20),
		MaxGoroutines:         getEnvInt("WS_MAX_GOROUTINES", 1000),

		// Safety thresholds (emergency brakes)
		CPURejectThreshold: getEnvFloat("WS_CPU_REJECT_THRESHOLD", 75.0),
		CPUPauseThreshold:  getEnvFloat("WS_CPU_PAUSE_THRESHOLD", 80.0),

		// JetStream configuration
		JSStreamMaxAge:    getEnvDuration("JS_STREAM_MAX_AGE", 30*time.Second),
		JSStreamMaxMsgs:   getEnvInt64("JS_STREAM_MAX_MSGS", 100000),
		JSStreamMaxBytes:  getEnvInt64("JS_STREAM_MAX_BYTES", 50*1024*1024), // 50MB
		JSConsumerAckWait: getEnvDuration("JS_CONSUMER_ACK_WAIT", 30*time.Second),
		JSStreamName:      getEnvString("JS_STREAM_NAME", "ODIN_TOKENS"),
		JSConsumerName:    getEnvString("JS_CONSUMER_NAME", "ws-server"),

		// Monitoring intervals
		MetricsInterval: getEnvDuration("METRICS_INTERVAL", 15*time.Second),

		// Logging configuration
		LogLevel:  LogLevel(getEnvString("LOG_LEVEL", "info")),
		LogFormat: LogFormat(getEnvString("LOG_FORMAT", "json")),
	}

	logger.Printf("Rate Limits:")
	logger.Printf("  NATS: %d msg/sec", config.MaxNATSMessagesPerSec)
	logger.Printf("  Broadcasts: %d/sec", config.MaxBroadcastsPerSec)
	logger.Printf("  Goroutines: %d max", config.MaxGoroutines)
	logger.Printf("Safety Thresholds:")
	logger.Printf("  CPU Reject: %.1f%%", config.CPURejectThreshold)
	logger.Printf("  CPU Pause: %.1f%%", config.CPUPauseThreshold)
	logger.Printf("JetStream:")
	logger.Printf("  Stream: %s (max age: %s)", config.JSStreamName, config.JSStreamMaxAge)
	logger.Printf("Logging:")
	logger.Printf("  Level: %s, Format: %s", config.LogLevel, config.LogFormat)

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
