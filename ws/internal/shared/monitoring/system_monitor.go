package monitoring

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/adred-codev/ws_poc/internal/shared/platform"
	"github.com/rs/zerolog"
)

// SystemMonitor is a singleton that centralizes system resource monitoring.
// It eliminates duplicate CPU/memory measurements across multiple ResourceGuards.
//
// Philosophy:
//   - Single source of truth for system metrics
//   - Measure once, query many times
//   - Zero duplication across shards/components
//   - Thread-safe concurrent access
//
// Benefits:
//   - CPU savings: N shards → 1 measurement instead of N measurements
//   - Consistent metrics: All components see same values
//   - Simplified architecture: Single monitoring goroutine
var (
	systemMonitorInstance *SystemMonitor
	systemMonitorOnce     sync.Once
)

// SystemMetrics holds current system resource measurements
type SystemMetrics struct {
	CPUPercent      float64           // Current CPU usage percentage (container-aware)
	MemoryBytes     int64             // Current memory usage in bytes
	MemoryMB        float64           // Current memory usage in MB
	Goroutines      int               // Current goroutine count
	CPUAllocation   float64           // CPU allocation (cores) from container limits
	ThrottleStats   platform.ThrottleStats // CPU throttling statistics
	Timestamp       time.Time         // When these metrics were captured
}

// SystemMonitor centralizes system resource monitoring
type SystemMonitor struct {
	cpuMonitor *platform.CPUMonitor
	logger     zerolog.Logger

	// Current metrics (protected by mutex)
	mu      sync.RWMutex
	metrics SystemMetrics

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// GetSystemMonitor returns the singleton SystemMonitor instance.
// First call initializes the monitor with the provided logger.
func GetSystemMonitor(logger zerolog.Logger) *SystemMonitor {
	systemMonitorOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())

		systemMonitorInstance = &SystemMonitor{
			cpuMonitor: platform.NewCPUMonitor(logger),
			logger:     logger.With().Str("component", "system_monitor").Logger(),
			ctx:        ctx,
			cancel:     cancel,
		}

		// Initialize metrics with zero values
		systemMonitorInstance.metrics = SystemMetrics{
			Timestamp: time.Now(),
		}

		logger.Info().
			Str("cpu_mode", systemMonitorInstance.cpuMonitor.Mode()).
			Float64("cpu_allocation", systemMonitorInstance.cpuMonitor.GetAllocation()).
			Msg("SystemMonitor singleton initialized")
	})

	return systemMonitorInstance
}

// StartMonitoring begins periodic system metric updates.
// Should be called once during application startup.
// Safe to call multiple times - only first call takes effect.
func (sm *SystemMonitor) StartMonitoring(interval time.Duration) {
	sm.wg.Add(1)
	go func() {
		defer sm.wg.Done()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		sm.logger.Info().
			Dur("interval", interval).
			Msg("SystemMonitor started")

		// Initial update
		sm.updateMetrics()

		for {
			select {
			case <-ticker.C:
				sm.updateMetrics()

			case <-sm.ctx.Done():
				sm.logger.Info().Msg("SystemMonitor stopped")
				return
			}
		}
	}()
}

// updateMetrics performs a single measurement of all system resources
func (sm *SystemMonitor) updateMetrics() {
	// Get container-aware CPU usage
	cpuPercent, throttleStats, err := sm.cpuMonitor.GetPercent()
	if err != nil {
		LogError(sm.logger, err, "Failed to get CPU usage", nil)
		cpuPercent = 0
	}

	// Get memory usage via ReadMemStats (proven reliable)
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	// Get goroutine count
	goroutines := runtime.NumGoroutine()

	// Update metrics atomically
	sm.mu.Lock()
	sm.metrics = SystemMetrics{
		CPUPercent:    cpuPercent,
		MemoryBytes:   int64(mem.Alloc),
		MemoryMB:      float64(mem.Alloc) / (1024 * 1024),
		Goroutines:    goroutines,
		CPUAllocation: sm.cpuMonitor.GetAllocation(),
		ThrottleStats: throttleStats,
		Timestamp:     time.Now(),
	}
	sm.mu.Unlock()

	// Update Prometheus metrics
	CpuUsagePercent.Set(cpuPercent)
	CpuContainerPercent.Set(cpuPercent)
	CpuAllocationCores.Set(sm.cpuMonitor.GetAllocation())

	// Also get host CPU for reference
	if hostCPU, err := sm.cpuMonitor.GetHostPercent(); err == nil {
		CpuHostPercent.Set(hostCPU)
	}

	// Update throttling metrics if available
	if throttleStats.NrThrottled > 0 {
		CpuThrottleEventsTotal.Add(float64(throttleStats.NrThrottled))
	}
	if throttleStats.ThrottledSec > 0 {
		CpuThrottledSecondsTotal.Add(throttleStats.ThrottledSec)
	}

	sm.logger.Debug().
		Float64("cpu_percent", cpuPercent).
		Uint64("cpu_throttled_events", throttleStats.NrThrottled).
		Float64("cpu_throttled_sec", throttleStats.ThrottledSec).
		Float64("memory_mb", sm.metrics.MemoryMB).
		Int("goroutines", goroutines).
		Msg("System metrics updated")
}

// GetMetrics returns a copy of the current system metrics.
// Thread-safe for concurrent access.
func (sm *SystemMonitor) GetMetrics() SystemMetrics {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.metrics
}

// GetCPUPercent returns the current CPU usage percentage.
// Convenience method for most common query.
func (sm *SystemMonitor) GetCPUPercent() float64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.metrics.CPUPercent
}

// GetMemoryBytes returns the current memory usage in bytes.
// Convenience method for memory checks.
func (sm *SystemMonitor) GetMemoryBytes() int64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.metrics.MemoryBytes
}

// GetMemoryMB returns the current memory usage in megabytes.
// Convenience method for health endpoints.
func (sm *SystemMonitor) GetMemoryMB() float64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.metrics.MemoryMB
}

// GetGoroutines returns the current goroutine count.
// Convenience method for debugging.
func (sm *SystemMonitor) GetGoroutines() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.metrics.Goroutines
}

// GetCPUAllocation returns the CPU allocation (cores) from container limits.
// Used for capacity planning and threshold calculations.
func (sm *SystemMonitor) GetCPUAllocation() float64 {
	return sm.cpuMonitor.GetAllocation()
}

// Shutdown gracefully stops the SystemMonitor.
// Should be called during application shutdown.
func (sm *SystemMonitor) Shutdown() {
	sm.logger.Info().Msg("Shutting down SystemMonitor")
	sm.cancel()
	sm.wg.Wait()
	sm.logger.Info().Msg("SystemMonitor shut down")
}
