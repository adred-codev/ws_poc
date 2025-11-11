package limits

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/adred-codev/ws_poc/internal/shared/monitoring"
	"github.com/adred-codev/ws_poc/internal/shared/platform"
	"github.com/adred-codev/ws_poc/internal/shared/types"
	"github.com/rs/zerolog"
	"golang.org/x/time/rate"
)

// ResourceGuard enforces static resource limits and prevents server overload.
//
// Philosophy:
//   - Static configuration (predictable behavior)
//   - Rate limiting (prevent work overload)
//   - Safety valves (emergency brakes)
//   - No auto-calculation (deterministic)
//
// Unlike DynamicCapacityManager, ResourceGuard does NOT:
//   - Calculate capacity from measurements
//   - Auto-adjust limits
//   - Track historical trends
//
// ResourceGuard DOES:
//   - Enforce configured limits strictly
//   - Rate limit Kafka consumption
//   - Rate limit broadcasts
//   - Provide safety checks (CPU, memory, goroutines)
//   - Log all decisions to Loki
type ResourceGuard struct {
	// Static configuration
	config types.ServerConfig
	logger zerolog.Logger

	// Rate limiters
	kafkaLimiter     *rate.Limiter // Limits Kafka message consumption
	broadcastLimiter *rate.Limiter // Limits broadcast operations

	// Goroutine limiter
	goroutineLimiter *GoroutineLimiter

	// CPU monitor (container-aware with fallback)
	cpuMonitor *platform.CPUMonitor

	// Current resource state (atomic)
	currentCPU    atomic.Value // float64
	currentMemory atomic.Value // int64 (bytes)

	// External state (pointers to server stats)
	currentConns *int64 // Pointer to server's current connection count (atomic operations used)
}

// GoroutineLimiter limits concurrent goroutines using a semaphore
type GoroutineLimiter struct {
	sem chan struct{}
	max int
}

// NewGoroutineLimiter creates a limiter that allows max concurrent goroutines
func NewGoroutineLimiter(max int) *GoroutineLimiter {
	return &GoroutineLimiter{
		sem: make(chan struct{}, max),
		max: max,
	}
}

// Acquire attempts to acquire a goroutine slot
// Returns true if acquired, false if at limit
func (gl *GoroutineLimiter) Acquire() bool {
	select {
	case gl.sem <- struct{}{}:
		return true
	default:
		return false
	}
}

// Release releases a goroutine slot
func (gl *GoroutineLimiter) Release() {
	<-gl.sem
}

// Current returns the current number of active goroutines
func (gl *GoroutineLimiter) Current() int {
	return len(gl.sem)
}

// Max returns the maximum allowed goroutines
func (gl *GoroutineLimiter) Max() int {
	return gl.max
}

// NewResourceGuard creates a new resource guard with static configuration
//
// Parameters:
//   - config: Server configuration with explicit resource limits
//   - logger: Structured logger for Loki
//   - currentConns: Pointer to server's current connection count (int64, uses atomic ops)
//
// Example:
//
//	guard := NewResourceGuard(config, logger, &server.stats.CurrentConnections)
func NewResourceGuard(config types.ServerConfig, logger zerolog.Logger, currentConns *int64) *ResourceGuard {
	// Create Kafka rate limiter
	// Limit: MaxKafkaMessagesPerSec per second
	// Burst: Allow up to 2x the rate in bursts (for traffic spikes)
	kafkaLimiter := rate.NewLimiter(
		rate.Limit(config.MaxKafkaMessagesPerSec),
		config.MaxKafkaMessagesPerSec*2, // Burst capacity
	)

	// Create broadcast rate limiter
	broadcastLimiter := rate.NewLimiter(
		rate.Limit(config.MaxBroadcastsPerSec),
		config.MaxBroadcastsPerSec*2,
	)

	// Create goroutine limiter
	goroutineLimiter := NewGoroutineLimiter(config.MaxGoroutines)

	// Initialize CPU monitor (container-aware with fallback)
	cpuMonitor := platform.NewCPUMonitor(logger)

	rg := &ResourceGuard{
		config:           config,
		logger:           logger,
		kafkaLimiter:     kafkaLimiter,
		broadcastLimiter: broadcastLimiter,
		goroutineLimiter: goroutineLimiter,
		cpuMonitor:       cpuMonitor,
		currentConns:     currentConns,
	}

	// Initialize atomic values
	rg.currentCPU.Store(0.0)
	rg.currentMemory.Store(int64(0))

	logger.Info().
		Str("cpu_mode", cpuMonitor.Mode()).
		Float64("cpu_allocation", cpuMonitor.GetAllocation()).
		Float64("cpu_limit", config.CPULimit).
		Int64("memory_limit", config.MemoryLimit).
		Int("max_connections", config.MaxConnections).
		Int("max_kafka_rate", config.MaxKafkaMessagesPerSec).
		Int("max_broadcast_rate", config.MaxBroadcastsPerSec).
		Int("max_goroutines", config.MaxGoroutines).
		Msgf("ResourceGuard initialized: %.1f CPUs allocated, will reject at %.0f%%",
			cpuMonitor.GetAllocation(),
			config.CPURejectThreshold)

	return rg
}

// ShouldAcceptConnection checks if a new connection can be accepted
//
// Checks (in order):
//  1. Hard connection limit
//  2. CPU emergency brake
//  3. Memory emergency brake
//  4. Goroutine limit
//
// Returns:
//   - accept: true if connection should be accepted
//   - reason: human-readable rejection reason (if rejected)
func (rg *ResourceGuard) ShouldAcceptConnection() (accept bool, reason string) {
	currentConns := atomic.LoadInt64(rg.currentConns)
	currentCPU := rg.currentCPU.Load().(float64)
	currentMemory := rg.currentMemory.Load().(int64)
	currentGoros := runtime.NumGoroutine()

	// Check 1: Hard connection limit
	if currentConns >= int64(rg.config.MaxConnections) {
		monitoring.IncrementCapacityRejection("at_max_connections")
		rg.logger.Debug().
			Int64("current_conns", currentConns).
			Int("max_conns", rg.config.MaxConnections).
			Msg("Connection rejected: at max connections")
		return false, fmt.Sprintf("at max connections (%d)", rg.config.MaxConnections)
	}

	// Check 2: CPU emergency brake
	if currentCPU > rg.config.CPURejectThreshold {
		monitoring.IncrementCapacityRejection("cpu_overload")
		rg.logger.Debug().
			Float64("current_cpu", currentCPU).
			Float64("threshold", rg.config.CPURejectThreshold).
			Msg("Connection rejected: CPU overload")
		return false, fmt.Sprintf("CPU %.1f%% > %.1f%%", currentCPU, rg.config.CPURejectThreshold)
	}

	// Check 3: Memory emergency brake
	if currentMemory > rg.config.MemoryLimit {
		monitoring.IncrementCapacityRejection("memory_limit")
		rg.logger.Debug().
			Int64("current_memory_mb", currentMemory/(1024*1024)).
			Int64("limit_mb", rg.config.MemoryLimit/(1024*1024)).
			Msg("Connection rejected: memory limit exceeded")
		return false, "memory limit exceeded"
	}

	// Check 4: Goroutine limit
	if currentGoros > rg.config.MaxGoroutines {
		monitoring.IncrementCapacityRejection("goroutine_limit")
		rg.logger.Debug().
			Int("current_goroutines", currentGoros).
			Int("max_goroutines", rg.config.MaxGoroutines).
			Msg("Connection rejected: goroutine limit exceeded")
		return false, fmt.Sprintf("goroutine limit exceeded (%d > %d)", currentGoros, rg.config.MaxGoroutines)
	}

	rg.logger.Debug().
		Int64("current_conns", currentConns).
		Float64("cpu", currentCPU).
		Int64("memory_mb", currentMemory/(1024*1024)).
		Int("goroutines", currentGoros).
		Msg("Connection accepted")

	return true, "OK"
}

// ShouldPauseKafka checks if Kafka consumption should be paused
//
// This provides backpressure when CPU is critically high.
// Consumer will pause partition consumption temporarily.
func (rg *ResourceGuard) ShouldPauseKafka() bool {
	currentCPU := rg.currentCPU.Load().(float64)
	return currentCPU > rg.config.CPUPauseThreshold
}

// AllowKafkaMessage checks if a Kafka message should be processed (rate limiting)
//
// This prevents Kafka from flooding the server with more work than it can handle.
//
// Returns:
//   - allow: true if message should be processed
//   - waitDuration: how long caller should wait before retrying (if blocked)
func (rg *ResourceGuard) AllowKafkaMessage(ctx context.Context) (allow bool, waitDuration time.Duration) {
	// Non-blocking check
	reservation := rg.kafkaLimiter.Reserve()
	if !reservation.OK() {
		// Rate limit exceeded
		rg.logger.Warn().Msg("Kafka rate limit exceeded")
		return false, 0
	}

	delay := reservation.Delay()
	if delay == 0 {
		// Allowed immediately
		return true, 0
	}

	// Would need to wait
	reservation.Cancel() // Don't consume token
	return false, delay
}

// AllowBroadcast checks if a broadcast should be processed (rate limiting)
func (rg *ResourceGuard) AllowBroadcast() bool {
	return rg.broadcastLimiter.Allow()
}

// AcquireGoroutine attempts to acquire permission to start a new goroutine
//
// Returns true if allowed, false if at limit.
// MUST call ReleaseGoroutine() when goroutine completes.
//
// Example:
//
//	if rg.AcquireGoroutine() {
//	    go func() {
//	        defer rg.ReleaseGoroutine()
//	        // ... work ...
//	    }()
//	}
func (rg *ResourceGuard) AcquireGoroutine() bool {
	acquired := rg.goroutineLimiter.Acquire()
	if !acquired {
		rg.logger.Warn().
			Int("current", rg.goroutineLimiter.Current()).
			Int("max", rg.goroutineLimiter.Max()).
			Msg("Goroutine limit reached")
	}
	return acquired
}

// ReleaseGoroutine releases a goroutine slot
func (rg *ResourceGuard) ReleaseGoroutine() {
	rg.goroutineLimiter.Release()
}

// UpdateResources updates current resource usage
//
// Call this periodically (e.g., every 15 seconds) to keep resource state current.
// Now also updates server stats to provide single source of truth.
func (rg *ResourceGuard) UpdateResources(serverStats *types.Stats) {
	// Get container-aware CPU usage
	cpuPercent, throttleStats, err := rg.cpuMonitor.GetPercent()
	if err != nil {
		monitoring.LogError(rg.logger, err, "Failed to get CPU usage", nil)
		cpuPercent = 0
	}

	// Store in ResourceGuard (for admission control)
	rg.currentCPU.Store(cpuPercent)

	// ALSO update server stats (for Prometheus/health endpoint)
	// This unifies the measurement - single source of truth
	if serverStats != nil {
		serverStats.Mu.Lock()
		serverStats.CPUPercent = cpuPercent
		serverStats.Mu.Unlock()
	}

	// Update throttling metrics if available
	if throttleStats.NrThrottled > 0 {
		monitoring.CpuThrottleEventsTotal.Add(float64(throttleStats.NrThrottled))
	}
	if throttleStats.ThrottledSec > 0 {
		monitoring.CpuThrottledSecondsTotal.Add(throttleStats.ThrottledSec)
	}

	// Get memory usage via ReadMemStats (proven reliable)
	// Why ReadMemStats instead of runtime/metrics:
	// - runtime/metrics returns KindBad on Docker/GCP environments (returns metric_kind=0)
	// - ReadMemStats is universal and reliable across all platforms
	// - Stop-the-world pause is < 1ms typically, acceptable for 15s update interval
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	rg.currentMemory.Store(int64(mem.Alloc))

	currentMemory := rg.currentMemory.Load().(int64)

	rg.logger.Debug().
		Float64("cpu_percent", cpuPercent).
		Uint64("cpu_throttled_events", throttleStats.NrThrottled).
		Float64("cpu_throttled_sec", throttleStats.ThrottledSec).
		Int64("memory_mb", currentMemory/(1024*1024)).
		Int64("connections", atomic.LoadInt64(rg.currentConns)).
		Int("goroutines", runtime.NumGoroutine()).
		Msg("Resource state updated")
}

// StartMonitoring begins periodic resource updates
func (rg *ResourceGuard) StartMonitoring(ctx context.Context, interval time.Duration, serverStats *types.Stats) {
	ticker := time.NewTicker(interval)

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				rg.UpdateResources(serverStats)

				// Update Prometheus metrics
				currentCPU := rg.currentCPU.Load().(float64)
				currentMemory := rg.currentMemory.Load().(int64)

				// Update container-aware CPU metrics
				monitoring.CpuUsagePercent.Set(currentCPU)
				monitoring.CpuContainerPercent.Set(currentCPU)
				monitoring.CpuAllocationCores.Set(rg.cpuMonitor.GetAllocation())

				// Also get host CPU for reference
				if hostCPU, err := rg.cpuMonitor.GetHostPercent(); err == nil {
					monitoring.CpuHostPercent.Set(hostCPU)
				}

				cpuHeadroom := 100.0 - currentCPU
				memPercent := 0.0
				if rg.config.MemoryLimit > 0 {
					memPercent = (float64(currentMemory) / float64(rg.config.MemoryLimit)) * 100
				}
				memHeadroom := 100.0 - memPercent

				monitoring.UpdateCapacityHeadroom(cpuHeadroom, memHeadroom)
				monitoring.UpdateCapacityMetrics(rg.config.MaxConnections, rg.config.CPURejectThreshold)

			case <-ctx.Done():
				rg.logger.Info().Msg("ResourceGuard monitoring stopped")
				return
			}
		}
	}()

	rg.logger.Info().
		Dur("interval", interval).
		Msg("ResourceGuard monitoring started")
}

// GetStats returns current resource statistics for debugging
func (rg *ResourceGuard) GetStats() map[string]any {
	return map[string]any{
		"max_connections":      rg.config.MaxConnections,
		"current_connections":  atomic.LoadInt64(rg.currentConns),
		"cpu_percent":          rg.currentCPU.Load().(float64),
		"cpu_reject_threshold": rg.config.CPURejectThreshold,
		"cpu_pause_threshold":  rg.config.CPUPauseThreshold,
		"memory_bytes":         rg.currentMemory.Load().(int64),
		"memory_limit_bytes":   rg.config.MemoryLimit,
		"goroutines_current":   runtime.NumGoroutine(),
		"goroutines_limit":     rg.config.MaxGoroutines,
		"kafka_rate_limit":     rg.config.MaxKafkaMessagesPerSec,
		"broadcast_rate_limit": rg.config.MaxBroadcastsPerSec,
	}
}
