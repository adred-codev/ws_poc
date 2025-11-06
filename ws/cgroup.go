package main

import (
	"os"
	"strconv"
	"strings"
)

// getMemoryLimit returns the container memory limit in bytes from cgroup filesystem.
//
// Purpose:
//
//	Automatically detect memory constraints in containerized environments
//	(Docker, Kubernetes, Cloud Run, ECS, etc.) to calculate safe connection limits.
//
// Supports:
//   - cgroup v2 (modern systems, Cloud Run, newer Kubernetes)
//   - cgroup v1 (legacy systems, older Docker versions)
//
// Return values:
//   - success: Returns memory limit in bytes
//   - no limit: Returns 0 (unlimited or non-containerized environment)
//   - error: Returns 0 with error (file not found, parse error)
//
// Implementation:
//
//	Tries cgroup v2 first (/sys/fs/cgroup/memory.max)
//	Falls back to cgroup v1 (/sys/fs/cgroup/memory/memory.limit_in_bytes)
//
// Example output:
//   - 512MB container: Returns 536870912 (512 * 1024 * 1024)
//   - Unlimited: Returns 0
//   - Non-containerized: Returns 0 with error
func getMemoryLimit() (int64, error) {
	// Try cgroup v2 first (newer systems, Cloud Run)
	// Path: /sys/fs/cgroup/memory.max
	// Format: "536870912" or "max" (unlimited)
	if data, err := os.ReadFile("/sys/fs/cgroup/memory.max"); err == nil {
		limitStr := strings.TrimSpace(string(data))
		if limitStr != "max" {
			return strconv.ParseInt(limitStr, 10, 64)
		}
	}

	// Fallback to cgroup v1 (legacy systems)
	// Path: /sys/fs/cgroup/memory/memory.limit_in_bytes
	// Format: "536870912" (always a number, never "max")
	if data, err := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes"); err == nil {
		limitStr := strings.TrimSpace(string(data))
		return strconv.ParseInt(limitStr, 10, 64)
	}

	// If no cgroup limits found, return 0 (no limit detected)
	// This happens on:
	//   - Non-containerized systems (bare metal, VMs)
	//   - macOS/Windows development environments
	//   - Containers without memory limits
	return 0, nil
}

// calculateMaxConnections determines safe max connections based on available memory.
//
// Memory breakdown per WebSocket connection (with reliability features):
//   - Client struct: ~200 bytes (basic fields, pointers, mutexes)
//   - send channel: 256 slots × 500 bytes avg = 128KB (buffered outgoing messages)
//   - replay buffer: 100 messages × 500 bytes avg = 50KB (message gap recovery)
//   - sequence generator: 8 bytes (atomic counter for message sequencing)
//   - Other overhead: ~2KB (connection pools, maps, alignment padding)
//     Total: ~180KB per connection
//
// Calculation example (512MB container):
//   - Container limit: 512MB
//   - Runtime overhead: 128MB (Go runtime, Kafka client, goroutine stacks)
//   - Available for connections: 384MB
//   - Max connections: 384MB / 180KB = ~2,133 connections
//
// Design tradeoff:
//
//	Fewer connections but with message reliability guarantees:
//	  - 100% message delivery (disconnect if can't deliver)
//	  - Gap recovery without full reconnect (better UX)
//	  - Production-ready for trading platforms
//
// Parameters:
//
//	memoryLimitBytes - Container memory limit from cgroup (0 = unlimited)
//
// Returns:
//
//	Safe maximum number of concurrent WebSocket connections
//
// Safety bounds:
//   - Minimum: 100 connections (viable service)
//   - Maximum: 50,000 connections (reasonable upper bound)
//   - Default: 10,000 connections (when no limit detected)
func calculateMaxConnections(memoryLimitBytes int64) int {
	if memoryLimitBytes == 0 {
		// No limit detected, use conservative default
		// Scenarios: bare metal, VMs, development environments
		return 10000
	}

	// Reserve 128MB for runtime overhead:
	//   - Go runtime heap: ~50MB
	//   - Kafka client: ~20MB
	//   - Goroutine stacks: ~30MB (2KB × 15,000 goroutines)
	//   - Buffer pools, metrics: ~10MB
	//   - Safety margin: ~18MB
	const runtimeOverheadBytes = 128 * 1024 * 1024

	// Average memory per connection with reliability features:
	//   - send channel: 128KB (256 slots × 500 bytes)
	//   - replay buffer: 50KB (100 messages × 500 bytes)
	//   - overhead: 2KB (struct, mutexes, pointers)
	const bytesPerConnection = 180 * 1024 // 180KB

	availableBytes := memoryLimitBytes - runtimeOverheadBytes
	if availableBytes < 0 {
		// Very constrained environment (e.g., 64MB container)
		// Use 50% of total memory for connections
		availableBytes = memoryLimitBytes / 2
	}

	maxConns := int(availableBytes / bytesPerConnection)

	// Apply safety bounds to prevent extreme configurations
	if maxConns < 100 {
		maxConns = 100 // Minimum viable service
	}
	if maxConns > 50000 {
		maxConns = 50000 // Maximum reasonable (network limits kick in)
	}

	return maxConns
}
