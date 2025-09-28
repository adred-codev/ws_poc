package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"odin-ws-server/internal/server"
	"odin-ws-server/internal/types"
)

const defaultConfig = `{
  "server": {
    "host": "0.0.0.0",
    "port": 3002,
    "readTimeout": 10,
    "writeTimeout": 10,
    "maxMessageSize": 1024
  },
  "websocket": {
    "checkOrigin": true,
    "enableCompression": true,
    "readBufferSize": 4096,
    "writeBufferSize": 4096,
    "handshakeTimeout": 10
  },
  "nats": {
    "url": "nats://localhost:4222",
    "maxReconnects": 10,
    "reconnectWait": 1000,
    "reconnectJitter": 200,
    "maxPingsOut": 3,
    "pingInterval": 10000
  },
  "auth": {
    "jwtSecret": "your-super-secret-jwt-key-change-in-production",
    "tokenExpiration": 3600,
    "requireAuth": false
  },
  "metrics": {
    "enablePrometheus": true,
    "metricsPath": "/metrics",
    "updateInterval": 1
  }
}`

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "", "Path to configuration file")
	flag.Parse()

	// Load configuration
	config, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create and start server
	srv, err := server.NewServer(config)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	if err := srv.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func loadConfig(configPath string) (*types.Config, error) {
	var configData []byte
	var err error

	if configPath != "" {
		// Load from file
		configData, err = os.ReadFile(configPath)
		if err != nil {
			return nil, err
		}
	} else {
		// Use default configuration
		configData = []byte(defaultConfig)
	}

	// Override with environment variables
	configData = []byte(expandEnvVars(string(configData)))

	var config types.Config
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, err
	}

	// Apply environment variable overrides
	applyEnvOverrides(&config)

	return &config, nil
}

func expandEnvVars(config string) string {
	// Simple environment variable expansion
	// In production, use a proper configuration library
	return os.ExpandEnv(config)
}

func applyEnvOverrides(config *types.Config) {
	// Server configuration
	if host := os.Getenv("SERVER_HOST"); host != "" {
		config.Server.Host = host
	}
	if port := os.Getenv("SERVER_PORT"); port != "" {
		// Parse port as integer (simplified for this example)
		switch port {
		case "3001":
			config.Server.Port = 3001
		case "3002":
			config.Server.Port = 3002
		case "8080":
			config.Server.Port = 8080
		}
	}

	// NATS configuration
	if natsURL := os.Getenv("NATS_URL"); natsURL != "" {
		config.NATS.URL = natsURL
	}

	// Auth configuration
	if jwtSecret := os.Getenv("JWT_SECRET"); jwtSecret != "" {
		config.Auth.JWTSecret = jwtSecret
	}
	if requireAuth := os.Getenv("REQUIRE_AUTH"); requireAuth == "true" {
		config.Auth.RequireAuth = true
	} else if requireAuth == "false" {
		config.Auth.RequireAuth = false
	}

	// Metrics configuration
	if enablePrometheus := os.Getenv("ENABLE_PROMETHEUS"); enablePrometheus == "false" {
		config.Metrics.EnablePrometheus = false
	} else if enablePrometheus == "true" {
		config.Metrics.EnablePrometheus = true
	}
}