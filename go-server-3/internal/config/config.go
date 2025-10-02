package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config holds all runtime configuration for the Go v3 WebSocket server.
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	WebSocket WebSocketConfig `mapstructure:"websocket"`
	Metrics   MetricsConfig   `mapstructure:"metrics"`
	Logging   LoggingConfig   `mapstructure:"logging"`
}

// ServerConfig contains network level settings for the HTTP/WebSocket listener.
type ServerConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	IdleTimeout     time.Duration `mapstructure:"idle_timeout"`
	ReadBufferSize  int           `mapstructure:"read_buffer_size"`
	WriteBufferSize int           `mapstructure:"write_buffer_size"`
}

// WebSocketConfig controls hub behaviour and connection limits.
type WebSocketConfig struct {
	Path              string `mapstructure:"path"`
	ShardCount        int    `mapstructure:"shard_count"`
	MaxConnections    int    `mapstructure:"max_connections"`
	SendChannelSize   int    `mapstructure:"send_channel_size"`
	BroadcastQueueSize int   `mapstructure:"broadcast_queue_size"`
	BroadcastWorkers   int   `mapstructure:"broadcast_workers"`
	EnableCompression bool   `mapstructure:"enable_compression"`
}

// MetricsConfig controls Prometheus/diagnostics endpoints.
type MetricsConfig struct {
	Enabled     bool   `mapstructure:"enabled"`
	ListenAddr  string `mapstructure:"listen_addr"`
	Endpoint    string `mapstructure:"endpoint"`
	ServiceName string `mapstructure:"service_name"`
}

// LoggingConfig controls zap logger level/encoding.
type LoggingConfig struct {
	Level       string `mapstructure:"level"`
	Development bool   `mapstructure:"development"`
}

// Load reads configuration from environment variables and optional config files.
func Load() (Config, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8082)
	v.SetDefault("server.read_timeout", 10*time.Second)
	v.SetDefault("server.write_timeout", 10*time.Second)
	v.SetDefault("server.idle_timeout", 120*time.Second)
	v.SetDefault("server.read_buffer_size", 16<<10)
	v.SetDefault("server.write_buffer_size", 16<<10)

	v.SetDefault("websocket.path", "/ws")
	v.SetDefault("websocket.shard_count", 64)
	v.SetDefault("websocket.max_connections", 100000)
	v.SetDefault("websocket.send_channel_size", 256)
	v.SetDefault("websocket.broadcast_queue_size", 1024)
	v.SetDefault("websocket.broadcast_workers", 0)
	v.SetDefault("websocket.enable_compression", false)

	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.listen_addr", ":9095")
	v.SetDefault("metrics.endpoint", "/metrics")
	v.SetDefault("metrics.service_name", "odin-go3")

	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.development", false)

	v.SetConfigName("odin")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	v.SetEnvPrefix("ODIN")
	v.AutomaticEnv()

	// Attempt to read config file (optional)
	_ = v.ReadInConfig()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("config unmarshal: %w", err)
	}

	if cfg.WebSocket.ShardCount <= 0 {
		cfg.WebSocket.ShardCount = 64
	}
	if cfg.WebSocket.SendChannelSize <= 0 {
		cfg.WebSocket.SendChannelSize = 256
	}

	return cfg, nil
}
