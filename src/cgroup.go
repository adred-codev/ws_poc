package main

import (
	"os"
	"strconv"
	"strings"
)

// getMemoryLimit returns the container memory limit in bytes
// Supports both cgroup v1 and v2
func getMemoryLimit() (int64, error) {
	// Try cgroup v2 first (newer systems, Cloud Run)
	if data, err := os.ReadFile("/sys/fs/cgroup/memory.max"); err == nil {
		limitStr := strings.TrimSpace(string(data))
		if limitStr != "max" {
			return strconv.ParseInt(limitStr, 10, 64)
		}
	}

	// Fallback to cgroup v1
	if data, err := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes"); err == nil {
		limitStr := strings.TrimSpace(string(data))
		return strconv.ParseInt(limitStr, 10, 64)
	}

	// If no cgroup limits found, return 0 (no limit detected)
	return 0, nil
}

// calculateMaxConnections determines safe max connections based on memory limit
//
// Memory breakdown per client (Phase 2 - with message reliability features):
// - Client struct: ~200 bytes (basic fields)
// - send channel: 256 slots × 500 bytes avg = 128KB (buffered outgoing messages)
// - replay buffer: 100 messages × 500 bytes avg = 50KB (gap recovery)
// - sequence generator: 8 bytes (atomic counter)
// - Other overhead: ~2KB (mutexes, pointers, alignment)
// Total: ~180KB per connection
//
// For 512MB container (Google Cloud Run):
// - Runtime overhead: 128MB (Go runtime, libraries, heap overhead)
// - Available for connections: 384MB
// - Max connections: 384MB / 180KB = ~2,133 connections
//
// This is significantly reduced from Phase 1 (7,864 connections with 50KB each)
// due to adding replay buffer for message reliability. Tradeoff is intentional:
// - Fewer connections, but 100% message delivery guarantee
// - Gap recovery without full reconnect (better UX)
// - Production-ready reliability for trading platform
//
// Alternative considered: Keep 7,864 limit but reduce buffer to 10 messages
// Rejected because: 10 messages = ~1 second coverage at 10msg/sec (too short)
func calculateMaxConnections(memoryLimitBytes int64) int {
	if memoryLimitBytes == 0 {
		// No limit detected, use conservative default
		return 10000
	}

	// Reserve 128MB for runtime, libraries, and overhead
	const runtimeOverheadBytes = 128 * 1024 * 1024

	// Average memory per connection (Phase 2 with reliability features)
	// Breakdown: send channel (128KB) + replay buffer (50KB) + overhead (2KB)
	const bytesPerConnection = 180 * 1024 // 180KB

	availableBytes := memoryLimitBytes - runtimeOverheadBytes
	if availableBytes < 0 {
		availableBytes = memoryLimitBytes / 2 // Use 50% if very limited
	}

	maxConns := int(availableBytes / bytesPerConnection)

	// Safety bounds
	if maxConns < 100 {
		maxConns = 100 // Minimum viable
	}
	if maxConns > 50000 {
		maxConns = 50000 // Maximum reasonable
	}

	return maxConns
}
