package metrics

import (
	"sync"
	"time"
)

// EnhancedMetrics provides comprehensive and accurate metrics collection
type EnhancedMetrics struct {
	// Use simple metrics instead of Prometheus
	simpleMetrics *SimpleMetrics

	systemMetrics     *SystemMetrics
	runtimeMetrics    *RuntimeMetricsReader
	cpuTracker        *CPUTracker
	connectionTracker *ConnectionTracker

	// Additional tracking
	mu              sync.RWMutex
	startTime       time.Time
	lastUpdateTime  time.Time
	updateInterval  time.Duration
}

// NewEnhancedMetrics creates a new enhanced metrics instance with simple metrics
func NewEnhancedMetrics() *EnhancedMetrics {
	return &EnhancedMetrics{
		simpleMetrics:     NewSimpleMetrics(),
		systemMetrics:     NewSystemMetrics(),
		runtimeMetrics:    NewRuntimeMetricsReader(),
		cpuTracker:        NewCPUTracker(),
		connectionTracker: NewConnectionTracker(),
		startTime:         time.Now(),
		lastUpdateTime:    time.Now(),
		updateInterval:    5 * time.Second,
	}
}

// StartCollection begins automatic metrics collection
func (em *EnhancedMetrics) StartCollection() {
	ticker := time.NewTicker(em.updateInterval)
	go func() {
		for range ticker.C {
			em.updateAllMetrics()
		}
	}()
}

// updateAllMetrics updates all metric types
func (em *EnhancedMetrics) updateAllMetrics() {
	em.mu.Lock()
	defer em.mu.Unlock()

	// Update system metrics (includes accurate CPU via gopsutil)
	em.systemMetrics.Update()

	// Update runtime metrics for detailed Go runtime stats
	em.runtimeMetrics.Update()

	// Sample CPU (keep as fallback)
	em.cpuTracker.Sample()

	// Update simple metrics instead of Prometheus
	// Use system metrics for accurate CPU, runtime metrics for memory
	em.simpleMetrics.UpdateMemoryUsage(uint64(em.systemMetrics.GetMemoryMB() * 1024 * 1024))
	em.simpleMetrics.UpdateCPUUsage(em.systemMetrics.GetCPUPercent())

	em.lastUpdateTime = time.Now()
}

// AddConnection tracks a new WebSocket connection
func (em *EnhancedMetrics) AddConnection(id, remoteAddr string) {
	em.simpleMetrics.IncrementConnections()
	em.connectionTracker.AddConnection(id, remoteAddr)
}

// RemoveConnection removes a tracked connection
func (em *EnhancedMetrics) RemoveConnection(id string) {
	em.simpleMetrics.DecrementConnections()
	em.connectionTracker.RemoveConnection(id)
}

// UpdateConnectionMessage updates message statistics for a connection
func (em *EnhancedMetrics) UpdateConnectionMessage(id string, sent bool, bytes int) {
	if sent {
		em.simpleMetrics.IncrementMessagesSent()
	} else {
		em.simpleMetrics.IncrementMessagesReceived()
	}

	em.simpleMetrics.RecordMessageSize(bytes)
	em.connectionTracker.UpdateConnectionStats(id, sent, uint64(bytes))
}

// GetAccurateStats returns comprehensive and accurate statistics
func (em *EnhancedMetrics) GetAccurateStats() map[string]interface{} {
	em.mu.RLock()
	defer em.mu.RUnlock()

	return map[string]interface{}{
		"timestamp": time.Now().Unix(),
		"uptime_seconds": time.Since(em.startTime).Seconds(),
		"last_update": em.lastUpdateTime.Unix(),

		// Connection metrics
		"connections": em.connectionTracker.GetConnectionStats(),

		// System metrics (now with accurate CPU via gopsutil)
		"system": map[string]interface{}{
			"memory": em.systemMetrics.GetMemoryStats(),
			"cpu": map[string]interface{}{
				"percent": em.systemMetrics.GetCPUPercent(), // Now accurate via gopsutil
				"cores":   em.systemMetrics.GetSystemInfo()["cpu"].(map[string]interface{})["cores"],
			},
		},

		// Enhanced runtime metrics from runtime/metrics
		"runtime": em.runtimeMetrics.GetAllStats(),

		// Performance metrics
		"performance": map[string]interface{}{
			"memory_mb":     em.systemMetrics.GetMemoryMB(),
			"cpu_percent":   em.systemMetrics.GetCPUPercent(), // Accurate CPU
			"goroutines":    em.systemMetrics.GetSystemInfo()["runtime"].(map[string]interface{})["goroutines"],
			"active_conns":  em.connectionTracker.GetActiveCount(),
		},
	}
}

// GetSimpleStats returns simplified metrics for the React client
func (em *EnhancedMetrics) GetSimpleStats() map[string]interface{} {
	// Combine simple metrics with enhanced connection tracking
	stats := em.simpleMetrics.GetSimpleStats()

	// Override with more accurate connection count
	if statsMap, ok := stats["connections"].(map[string]interface{}); ok {
		statsMap["active"] = em.connectionTracker.GetActiveCount()
	}

	return stats
}