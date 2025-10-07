package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
)

// DynamicCapacityManager calculates and adjusts connection limits based on real-time resource availability
//
// Key principles:
//   1. Measure actual resource usage, don't assume
//   2. Detect container/cgroup limits (cloud-native)
//   3. Account for co-located services dynamically
//   4. Adjust capacity based on observed performance
//   5. Fail-safe: Conservative limits when uncertain
//
// Design philosophy:
//   - Start conservative, increase when proven safe
//   - Monitor continuously, adjust based on config
//   - Prefer stability over maximum utilization
//   - Never exceed configured CPU target
type DynamicCapacityManager struct {
	mu sync.RWMutex

	// Configuration
	config ServerConfig

	// Resource measurements
	totalCPU         int     // Total CPU cores available
	availableMemory  int64   // Available memory in bytes (from cgroup)
	ourPID           int32   // Our process ID for resource monitoring

	// Performance metrics (measured at runtime)
	avgBroadcastTimeMs float64 // Average time to broadcast to all clients
	avgCPUPercent      float64 // Average CPU usage over last minute
	avgMemoryBytes     int64   // Average memory usage over last minute

	// Current capacity limits
	maxConnections     int     // Current max connections allowed
	cpuThresholdReject float64 // Reject new connections above this CPU %
	cpuThresholdPause  float64 // Pause NATS consumption above this CPU %

	// Historical data for trend analysis
	measurements       []measurement
	maxMeasurements    int

	logger *log.Logger
}

type measurement struct {
	timestamp       time.Time
	connections     int64
	cpuPercent      float64
	memoryBytes     int64
	broadcastTimeMs float64
}

// NewDynamicCapacityManager creates a capacity manager that adapts to real resource availability
func NewDynamicCapacityManager(config ServerConfig, logger *log.Logger) (*DynamicCapacityManager, error) {
	// Get our process ID for resource monitoring
	pid := int32(os.Getpid())

	// Detect CPU cores (respects cgroup limits in containers)
	totalCPU := runtime.GOMAXPROCS(0)

	// Get memory limit from cgroup (container-aware)
	memLimit, err := getMemoryLimit()
	if err != nil {
		logger.Printf("⚠️  Failed to get memory limit, using conservative default: %v", err)
		memLimit = 256 * 1024 * 1024 // 256MB conservative default
	}

	dcm := &DynamicCapacityManager{
		config:             config,
		totalCPU:           totalCPU,
		availableMemory:    memLimit,
		ourPID:             pid,
		maxMeasurements:    120, // Keep 1 hour of history
		measurements:       make([]measurement, 0, 120),
		logger:             logger,

		// Use configured thresholds
		cpuThresholdReject: config.CPURejectThreshold,
		cpuThresholdPause:  config.CPUPauseThreshold,
		maxConnections:     config.MinConnections, // Start at minimum
	}

	// Calculate initial capacity estimate
	dcm.recalculateCapacity()

	logger.Printf("🔧 Dynamic Capacity Manager initialized:")
	logger.Printf("   Total CPU cores: %d", totalCPU)
	logger.Printf("   Available memory: %.2f MB", float64(memLimit)/(1024*1024))
	logger.Printf("   Initial max connections: %d", dcm.maxConnections)
	logger.Printf("   CPU reject threshold: %.1f%%", dcm.cpuThresholdReject)
	logger.Printf("   CPU pause threshold: %.1f%%", dcm.cpuThresholdPause)

	return dcm, nil
}

// GetMaxConnections returns the current safe connection limit
func (dcm *DynamicCapacityManager) GetMaxConnections() int {
	dcm.mu.RLock()
	defer dcm.mu.RUnlock()
	return dcm.maxConnections
}

// ShouldAcceptConnection checks if we can safely accept a new connection
// Returns (shouldAccept bool, reason string, cpuPercent float64)
func (dcm *DynamicCapacityManager) ShouldAcceptConnection(currentConnections int64, currentCPU float64) (bool, string, float64) {
	dcm.mu.RLock()
	defer dcm.mu.RUnlock()

	// Check 1: CPU overload
	if currentCPU > dcm.cpuThresholdReject {
		IncrementCapacityRejection("cpu_overload")
		return false, fmt.Sprintf("CPU too high: %.1f%% > %.1f%%", currentCPU, dcm.cpuThresholdReject), currentCPU
	}

	// Check 2: At capacity limit
	if currentConnections >= int64(dcm.maxConnections) {
		IncrementCapacityRejection("at_capacity")
		return false, fmt.Sprintf("At capacity: %d >= %d", currentConnections, dcm.maxConnections), currentCPU
	}

	return true, "OK", currentCPU
}

// ShouldPauseNATS checks if we should pause consuming from NATS due to high load
func (dcm *DynamicCapacityManager) ShouldPauseNATS(currentCPU float64) bool {
	dcm.mu.RLock()
	defer dcm.mu.RUnlock()
	return currentCPU > dcm.cpuThresholdPause
}

// RecordMeasurement records current performance metrics for capacity calculation
func (dcm *DynamicCapacityManager) RecordMeasurement(connections int64, cpuPercent float64, memoryBytes int64, broadcastTimeMs float64) {
	dcm.mu.Lock()
	defer dcm.mu.Unlock()

	m := measurement{
		timestamp:       time.Now(),
		connections:     connections,
		cpuPercent:      cpuPercent,
		memoryBytes:     memoryBytes,
		broadcastTimeMs: broadcastTimeMs,
	}

	dcm.measurements = append(dcm.measurements, m)

	// Keep only last N measurements
	if len(dcm.measurements) > dcm.maxMeasurements {
		dcm.measurements = dcm.measurements[1:]
	}

	// Update averages
	dcm.updateAverages()
}

// updateAverages calculates rolling averages from recent measurements
func (dcm *DynamicCapacityManager) updateAverages() {
	if len(dcm.measurements) == 0 {
		return
	}

	// Use last 10 measurements (5 minutes) for averages
	start := len(dcm.measurements) - 10
	if start < 0 {
		start = 0
	}

	recent := dcm.measurements[start:]

	var totalCPU, totalMem, totalBroadcast float64
	for _, m := range recent {
		totalCPU += m.cpuPercent
		totalMem += float64(m.memoryBytes)
		totalBroadcast += m.broadcastTimeMs
	}

	count := float64(len(recent))
	dcm.avgCPUPercent = totalCPU / count
	dcm.avgMemoryBytes = int64(totalMem / count)
	dcm.avgBroadcastTimeMs = totalBroadcast / count
}

// RecalculateCapacity adjusts connection limits based on observed performance
func (dcm *DynamicCapacityManager) RecalculateCapacity() {
	dcm.mu.Lock()
	defer dcm.mu.Unlock()
	dcm.recalculateCapacity()
}

// recalculateCapacity (internal, assumes lock held)
func (dcm *DynamicCapacityManager) recalculateCapacity() {
	// Step 1: Calculate CPU-based capacity
	cpuCapacity := dcm.calculateCPUCapacity()

	// Step 2: Calculate memory-based capacity
	memCapacity := dcm.calculateMemoryCapacity()

	// Step 3: Take the minimum (most conservative)
	newCapacity := int(math.Min(float64(cpuCapacity), float64(memCapacity)))

	// Step 4: Apply configured safety margin
	newCapacity = int(float64(newCapacity) * dcm.config.SafetyMargin)

	// Step 5: Enforce configured bounds
	if newCapacity < dcm.config.MinConnections {
		newCapacity = dcm.config.MinConnections
	}
	if newCapacity > dcm.config.MaxCapacity {
		newCapacity = dcm.config.MaxCapacity
	}

	// Only log if capacity changed significantly (>10%)
	oldCapacity := dcm.maxConnections
	changePct := math.Abs(float64(newCapacity-oldCapacity)) / float64(oldCapacity) * 100

	if changePct > 10 || oldCapacity == 100 { // Always log first real calculation
		dcm.logger.Printf("📊 Capacity recalculated: %d → %d (CPU: %d, Mem: %d)",
			oldCapacity, newCapacity, cpuCapacity, memCapacity)
	}

	dcm.maxConnections = newCapacity

	// Update Prometheus metrics
	UpdateCapacityMetrics(newCapacity, dcm.cpuThresholdReject)
}

// calculateCPUCapacity estimates max connections based on CPU resources
func (dcm *DynamicCapacityManager) calculateCPUCapacity() int {
	// Get system-wide CPU usage to detect other processes
	systemCPU, err := cpu.Percent(1*time.Second, false)
	if err != nil || len(systemCPU) == 0 {
		// Fallback: assume 50% CPU available
		return dcm.totalCPU * 250 // 250 connections per CPU core
	}

	systemCPUPercent := systemCPU[0]

	// Calculate CPU headroom: how much CPU is still available
	// Use configured target maximum CPU
	targetMaxCPU := dcm.config.CPUTargetMax
	currentUsed := systemCPUPercent
	availableHeadroom := targetMaxCPU - currentUsed

	if availableHeadroom < 10 {
		availableHeadroom = 10 // Always leave at least 10% headroom
	}

	// Estimate connections per CPU percent based on measurements
	if len(dcm.measurements) > 5 {
		// We have data - calculate actual efficiency
		// Look for stable measurements (not ramping up/down)
		var stableMeasurements []measurement
		for _, m := range dcm.measurements {
			if m.connections > 100 && m.cpuPercent > 5 {
				stableMeasurements = append(stableMeasurements, m)
			}
		}

		if len(stableMeasurements) > 0 {
			// Calculate observed connections per CPU percent
			var totalRatio float64
			for _, m := range stableMeasurements {
				totalRatio += float64(m.connections) / m.cpuPercent
			}
			avgConnectionsPerCPUPercent := totalRatio / float64(len(stableMeasurements))

			// Calculate capacity based on available headroom
			capacity := int(avgConnectionsPerCPUPercent * availableHeadroom)

			dcm.logger.Printf("📈 CPU capacity (measured): %.1f conn/cpu%% × %.1f%% headroom = %d",
				avgConnectionsPerCPUPercent, availableHeadroom, capacity)

			return capacity
		}
	}

	// No measurements yet - use conservative estimate
	// Assume 10 connections per CPU percent (very conservative)
	capacity := int(10 * availableHeadroom * float64(dcm.totalCPU))

	dcm.logger.Printf("📈 CPU capacity (estimated): %d cores × %.1f%% headroom = %d",
		dcm.totalCPU, availableHeadroom, capacity)

	return capacity
}

// calculateMemoryCapacity estimates max connections based on memory resources
func (dcm *DynamicCapacityManager) calculateMemoryCapacity() int {
	// Get current memory usage
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	currentMemory := int64(mem.Alloc)

	// Reserve memory for:
	// 1. Current baseline usage (our process without connections)
	// 2. Other services (if co-located)
	// 3. Headroom for spikes

	baselineMemory := int64(128 * 1024 * 1024) // 128MB baseline
	if currentMemory > baselineMemory {
		baselineMemory = currentMemory
	}

	// Reserve 20% for headroom and other services
	reservedMemory := int64(float64(dcm.availableMemory) * 0.2)
	usableMemory := dcm.availableMemory - baselineMemory - reservedMemory

	if usableMemory < 0 {
		dcm.logger.Printf("⚠️  Memory capacity warning: usable memory is negative, using minimum")
		return 50 // Minimum safe value
	}

	// Estimate bytes per connection
	bytesPerConnection := int64(180 * 1024) // 180KB (send buffer + replay buffer)

	// If we have measurements with actual memory usage, calculate real cost
	if len(dcm.measurements) > 10 {
		// Find measurements with significant connections
		var validSamples []measurement
		for i := 1; i < len(dcm.measurements); i++ {
			prev := dcm.measurements[i-1]
			curr := dcm.measurements[i]

			// Look for cases where connections increased
			connDelta := curr.connections - prev.connections
			memDelta := curr.memoryBytes - prev.memoryBytes

			if connDelta > 10 && memDelta > 0 {
				validSamples = append(validSamples, measurement{
					connections: connDelta,
					memoryBytes: memDelta,
				})
			}
		}

		if len(validSamples) > 3 {
			// Calculate actual bytes per connection from samples
			var totalBytesPerConn int64
			for _, s := range validSamples {
				totalBytesPerConn += s.memoryBytes / s.connections
			}
			measuredBytesPerConn := totalBytesPerConn / int64(len(validSamples))

			// Use measured value if reasonable (between 100KB and 500KB)
			if measuredBytesPerConn >= 100*1024 && measuredBytesPerConn <= 500*1024 {
				bytesPerConnection = measuredBytesPerConn
				dcm.logger.Printf("💾 Using measured memory/conn: %.1f KB", float64(bytesPerConnection)/1024)
			}
		}
	}

	capacity := int(usableMemory / bytesPerConnection)

	dcm.logger.Printf("💾 Memory capacity: %.1f MB usable ÷ %.1f KB/conn = %d",
		float64(usableMemory)/(1024*1024),
		float64(bytesPerConnection)/1024,
		capacity)

	return capacity
}

// StartMonitoring begins periodic capacity recalculation
func (dcm *DynamicCapacityManager) StartMonitoring(server *Server) {
	ticker := time.NewTicker(dcm.config.CapacityInterval)

	go func() {
		defer ticker.Stop()

		for range ticker.C {
			// Collect current metrics
			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)

			currentConnections := server.stats.CurrentConnections

			server.stats.mu.RLock()
			currentCPU := server.stats.CPUPercent
			server.stats.mu.RUnlock()

			// Record measurement (broadcast time would need to be tracked separately)
			dcm.RecordMeasurement(
				currentConnections,
				currentCPU,
				int64(mem.Alloc),
				0, // TODO: Track actual broadcast time
			)

			// Calculate and update resource headroom metrics
			cpuHeadroom := 100.0 - currentCPU
			memUsedPct := float64(mem.Alloc) / float64(dcm.availableMemory) * 100
			memHeadroom := 100.0 - memUsedPct

			UpdateCapacityHeadroom(cpuHeadroom, memHeadroom)

			// Recalculate capacity based on new data
			dcm.RecalculateCapacity()
		}
	}()

	dcm.logger.Printf("📊 Dynamic capacity monitoring started (%s intervals)", dcm.config.CapacityInterval)
}

// GetStats returns current capacity stats for monitoring/debugging
func (dcm *DynamicCapacityManager) GetStats() map[string]interface{} {
	dcm.mu.RLock()
	defer dcm.mu.RUnlock()

	return map[string]interface{}{
		"maxConnections":     dcm.maxConnections,
		"cpuThresholdReject": dcm.cpuThresholdReject,
		"cpuThresholdPause":  dcm.cpuThresholdPause,
		"avgCPUPercent":      dcm.avgCPUPercent,
		"avgMemoryMB":        float64(dcm.avgMemoryBytes) / (1024 * 1024),
		"totalCPU":           dcm.totalCPU,
		"availableMemoryMB":  float64(dcm.availableMemory) / (1024 * 1024),
		"measurementCount":   len(dcm.measurements),
	}
}
