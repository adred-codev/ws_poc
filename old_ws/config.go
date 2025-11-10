package main

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
)

// Config holds all server configuration
// Tags:
//
//	env: Environment variable name
//	envDefault: Default value if not set
//	required: Must be provided (no default)
type Config struct {
	// Server basics
	Addr    string `env:"WS_ADDR" envDefault:":3002"`
	NATSUrl string `env:"NATS_URL" envDefault:""`

	// Resource limits (from container)
	CPULimit    float64 `env:"WS_CPU_LIMIT" envDefault:"1.0"`
	MemoryLimit int64   `env:"WS_MEMORY_LIMIT" envDefault:"536870912"` // 512MB

	// Capacity
	MaxConnections int `env:"WS_MAX_CONNECTIONS" envDefault:"500"`

	// Worker pool (computed from CPU if not set)
	WorkerPoolSize  int `env:"WS_WORKER_POOL_SIZE" envDefault:"0"`  // 0 = auto-calculate
	WorkerQueueSize int `env:"WS_WORKER_QUEUE_SIZE" envDefault:"0"` // 0 = auto-calculate

	// Rate limiting
	MaxNATSRate      int `env:"WS_MAX_NATS_RATE" envDefault:"20"`
	MaxBroadcastRate int `env:"WS_MAX_BROADCAST_RATE" envDefault:"20"`
	MaxGoroutines    int `env:"WS_MAX_GOROUTINES" envDefault:"1000"`

	// Safety thresholds
	CPURejectThreshold float64 `env:"WS_CPU_REJECT_THRESHOLD" envDefault:"75.0"`
	CPUPauseThreshold  float64 `env:"WS_CPU_PAUSE_THRESHOLD" envDefault:"80.0"`

	// JetStream
	JSStreamMaxAge    time.Duration `env:"JS_STREAM_MAX_AGE" envDefault:"30s"`
	JSStreamMaxMsgs   int64         `env:"JS_STREAM_MAX_MSGS" envDefault:"100000"`
	JSStreamMaxBytes  int64         `env:"JS_STREAM_MAX_BYTES" envDefault:"52428800"` // 50MB
	JSConsumerAckWait time.Duration `env:"JS_CONSUMER_ACK_WAIT" envDefault:"30s"`
	JSStreamName      string        `env:"JS_STREAM_NAME" envDefault:"ODIN_TOKENS"`
	JSConsumerName    string        `env:"JS_CONSUMER_NAME" envDefault:"ws-server"`

	// Monitoring
	MetricsInterval time.Duration `env:"METRICS_INTERVAL" envDefault:"15s"`

	// Logging
	LogLevel  string `env:"LOG_LEVEL" envDefault:"info"`
	LogFormat string `env:"LOG_FORMAT" envDefault:"json"`

	// Environment
	Environment string `env:"ENVIRONMENT" envDefault:"development"`
}

// LoadConfig reads configuration from .env file and environment variables
// Priority: ENV vars > .env file > defaults
//
// Optional logger parameter for structured logging. If nil, logs to stdout.
func LoadConfig(logger *zerolog.Logger) (*Config, error) {
	// Load .env file (optional - OK if it doesn't exist)
	// In production (Docker), we use environment variables directly
	// In development, .env file provides convenience
	if err := godotenv.Load(); err != nil {
		// Only log, don't fail - we can run without .env file
		if logger != nil {
			logger.Info().Msg("No .env file found (using environment variables only)")
		} else {
			fmt.Println("Info: No .env file found (using environment variables only)")
		}
	} else {
		if logger != nil {
			logger.Info().Msg("Loaded configuration from .env file")
		}
	}

	cfg := &Config{}

	// Parse environment variables into struct
	// This validates types and applies defaults
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Post-processing: Auto-calculate worker pool if not set
	if cfg.WorkerPoolSize == 0 {
		cfg.WorkerPoolSize = int(cfg.CPULimit * 2)
		if logger != nil {
			logger.Info().
				Float64("cpu_limit", cfg.CPULimit).
				Int("worker_pool_size", cfg.WorkerPoolSize).
				Msg("Auto-calculated worker pool size from CPU limit")
		}
	}
	if cfg.WorkerQueueSize == 0 {
		cfg.WorkerQueueSize = cfg.WorkerPoolSize * 100
		if logger != nil {
			logger.Info().
				Int("worker_pool_size", cfg.WorkerPoolSize).
				Int("worker_queue_size", cfg.WorkerQueueSize).
				Msg("Auto-calculated worker queue size")
		}
	}

	// Validation
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	if logger != nil {
		logger.Info().Msg("Configuration loaded and validated successfully")
	}

	return cfg, nil
}

// Validate checks configuration for errors
func (c *Config) Validate() error {
	// Required fields (no sensible defaults)
	if c.Addr == "" {
		return fmt.Errorf("WS_ADDR is required")
	}

	// Range checks
	if c.MaxConnections < 1 {
		return fmt.Errorf("WS_MAX_CONNECTIONS must be > 0, got %d", c.MaxConnections)
	}
	if c.WorkerPoolSize < 1 {
		return fmt.Errorf("WS_WORKER_POOL_SIZE must be > 0, got %d", c.WorkerPoolSize)
	}
	if c.CPURejectThreshold < 0 || c.CPURejectThreshold > 100 {
		return fmt.Errorf("WS_CPU_REJECT_THRESHOLD must be 0-100, got %.1f", c.CPURejectThreshold)
	}
	if c.CPUPauseThreshold < 0 || c.CPUPauseThreshold > 100 {
		return fmt.Errorf("WS_CPU_PAUSE_THRESHOLD must be 0-100, got %.1f", c.CPUPauseThreshold)
	}

	// Logical checks
	if c.CPUPauseThreshold < c.CPURejectThreshold {
		return fmt.Errorf("WS_CPU_PAUSE_THRESHOLD (%.1f) must be >= WS_CPU_REJECT_THRESHOLD (%.1f)",
			c.CPUPauseThreshold, c.CPURejectThreshold)
	}

	// Enum checks
	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[c.LogLevel] {
		return fmt.Errorf("LOG_LEVEL must be one of: debug, info, warn, error (got: %s)", c.LogLevel)
	}

	validLogFormats := map[string]bool{"json": true, "text": true, "pretty": true}
	if !validLogFormats[c.LogFormat] {
		return fmt.Errorf("LOG_FORMAT must be one of: json, text, pretty (got: %s)", c.LogFormat)
	}

	return nil
}

// Print logs configuration for debugging (human-readable format)
// For production, use LogConfig() with structured logging
func (c *Config) Print() {
	fmt.Println("=== Server Configuration ===")
	fmt.Printf("Environment:     %s\n", c.Environment)
	fmt.Printf("Address:         %s\n", c.Addr)
	fmt.Printf("NATS URL:        %s\n", c.NATSUrl)
	fmt.Println("\n=== Resource Limits ===")
	fmt.Printf("CPU Limit:       %.1f cores\n", c.CPULimit)
	fmt.Printf("Memory Limit:    %d MB\n", c.MemoryLimit/(1024*1024))
	fmt.Printf("Max Connections: %d\n", c.MaxConnections)
	fmt.Println("\n=== Worker Pool ===")
	fmt.Printf("Workers:         %d\n", c.WorkerPoolSize)
	fmt.Printf("Queue Size:      %d\n", c.WorkerQueueSize)
	fmt.Println("\n=== Rate Limits ===")
	fmt.Printf("NATS Messages:   %d/sec\n", c.MaxNATSRate)
	fmt.Printf("Broadcasts:      %d/sec\n", c.MaxBroadcastRate)
	fmt.Printf("Max Goroutines:  %d\n", c.MaxGoroutines)
	fmt.Println("\n=== Safety Thresholds ===")
	fmt.Printf("CPU Reject:      %.1f%%\n", c.CPURejectThreshold)
	fmt.Printf("CPU Pause:       %.1f%%\n", c.CPUPauseThreshold)
	fmt.Println("\n=== JetStream ===")
	fmt.Printf("Stream:          %s\n", c.JSStreamName)
	fmt.Printf("Consumer:        %s\n", c.JSConsumerName)
	fmt.Printf("Max Age:         %s\n", c.JSStreamMaxAge)
	fmt.Printf("Max Messages:    %d\n", c.JSStreamMaxMsgs)
	fmt.Printf("Max Bytes:       %d MB\n", c.JSStreamMaxBytes/(1024*1024))
	fmt.Println("\n=== Logging ===")
	fmt.Printf("Level:           %s\n", c.LogLevel)
	fmt.Printf("Format:          %s\n", c.LogFormat)
	fmt.Println("============================")
}

// LogConfig logs configuration using structured logging (Loki-compatible)
func (c *Config) LogConfig(logger zerolog.Logger) {
	logger.Info().
		Str("environment", c.Environment).
		Str("addr", c.Addr).
		Str("nats_url", c.NATSUrl).
		Float64("cpu_limit", c.CPULimit).
		Int64("memory_limit_mb", c.MemoryLimit/(1024*1024)).
		Int("max_connections", c.MaxConnections).
		Int("worker_pool_size", c.WorkerPoolSize).
		Int("worker_queue_size", c.WorkerQueueSize).
		Int("max_nats_rate", c.MaxNATSRate).
		Int("max_broadcast_rate", c.MaxBroadcastRate).
		Int("max_goroutines", c.MaxGoroutines).
		Float64("cpu_reject_threshold", c.CPURejectThreshold).
		Float64("cpu_pause_threshold", c.CPUPauseThreshold).
		Str("js_stream_name", c.JSStreamName).
		Str("js_consumer_name", c.JSConsumerName).
		Dur("js_stream_max_age", c.JSStreamMaxAge).
		Int64("js_stream_max_msgs", c.JSStreamMaxMsgs).
		Int64("js_stream_max_bytes_mb", c.JSStreamMaxBytes/(1024*1024)).
		Dur("js_consumer_ack_wait", c.JSConsumerAckWait).
		Dur("metrics_interval", c.MetricsInterval).
		Str("log_level", c.LogLevel).
		Str("log_format", c.LogFormat).
		Msg("Server configuration loaded")
}
