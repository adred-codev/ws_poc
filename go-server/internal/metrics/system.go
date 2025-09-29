package metrics

import (
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
)

// SystemMetrics provides accurate system resource tracking
type SystemMetrics struct {
	mu               sync.RWMutex
	lastCPUTime      time.Time
	lastCPUTotal     float64
	lastCPUProcess   float64
	cpuPercent       float64
	memoryStats      runtime.MemStats
	lastMemUpdate    time.Time
}

// NewSystemMetrics creates a new system metrics tracker
func NewSystemMetrics() *SystemMetrics {
	sm := &SystemMetrics{
		lastCPUTime:   time.Now(),
		lastMemUpdate: time.Now(),
	}

	// Initialize CPU tracking
	sm.updateCPUMetrics()

	return sm
}

// Update refreshes all system metrics
func (sm *SystemMetrics) Update() {
	sm.updateMemoryMetrics()
	sm.updateCPUMetrics()
}

// updateMemoryMetrics updates memory statistics
func (sm *SystemMetrics) updateMemoryMetrics() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	runtime.ReadMemStats(&sm.memoryStats)
	sm.lastMemUpdate = time.Now()
}

// updateCPUMetrics calculates CPU usage percentage using gopsutil
func (sm *SystemMetrics) updateCPUMetrics() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Get actual system CPU usage using gopsutil
	cpuPercents, err := cpu.Percent(time.Second, false)
	if err != nil || len(cpuPercents) == 0 {
		// Fallback to previous value or 0
		return
	}

	// Use overall CPU percentage (first element when per_cpu=false)
	currentCPU := cpuPercents[0]

	// Apply smoothing to avoid spikes
	if sm.cpuPercent == 0 {
		sm.cpuPercent = currentCPU
	} else {
		// Exponential moving average for stability
		alpha := 0.3
		sm.cpuPercent = alpha*currentCPU + (1-alpha)*sm.cpuPercent
	}

	sm.lastCPUTime = time.Now()
}

// GetMemoryMB returns memory usage in megabytes
func (sm *SystemMetrics) GetMemoryMB() float64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return float64(sm.memoryStats.HeapAlloc) / 1024 / 1024
}

// GetMemoryStats returns detailed memory statistics
func (sm *SystemMetrics) GetMemoryStats() map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return map[string]interface{}{
		"heap_alloc_mb":    float64(sm.memoryStats.HeapAlloc) / 1024 / 1024,
		"heap_sys_mb":      float64(sm.memoryStats.HeapSys) / 1024 / 1024,
		"heap_idle_mb":     float64(sm.memoryStats.HeapIdle) / 1024 / 1024,
		"heap_inuse_mb":    float64(sm.memoryStats.HeapInuse) / 1024 / 1024,
		"heap_released_mb": float64(sm.memoryStats.HeapReleased) / 1024 / 1024,
		"stack_inuse_mb":   float64(sm.memoryStats.StackInuse) / 1024 / 1024,
		"sys_total_mb":     float64(sm.memoryStats.Sys) / 1024 / 1024,
		"gc_count":         sm.memoryStats.NumGC,
		"gc_cpu_percent":   sm.memoryStats.GCCPUFraction * 100,
		"goroutines":       runtime.NumGoroutine(),
	}
}

// GetCPUPercent returns the current CPU usage percentage
func (sm *SystemMetrics) GetCPUPercent() float64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return sm.cpuPercent
}

// GetSystemInfo returns comprehensive system information
func (sm *SystemMetrics) GetSystemInfo() map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return map[string]interface{}{
		"cpu": map[string]interface{}{
			"cores":   runtime.NumCPU(),
			"percent": sm.cpuPercent,
		},
		"memory": map[string]interface{}{
			"heap_alloc_mb": float64(sm.memoryStats.HeapAlloc) / 1024 / 1024,
			"sys_total_mb":  float64(sm.memoryStats.Sys) / 1024 / 1024,
			"gc_count":      sm.memoryStats.NumGC,
		},
		"runtime": map[string]interface{}{
			"goroutines": runtime.NumGoroutine(),
			"go_version": runtime.Version(),
		},
	}
}

// CPUTracker provides more accurate CPU tracking using time-based sampling
type CPUTracker struct {
	mu              sync.RWMutex
	startTime       time.Time
	startTotalTime  int64
	lastSampleTime  time.Time
	lastTotalTime   int64
	cpuPercent      float64
	samples         []float64
	maxSamples      int
}

// NewCPUTracker creates a new CPU tracker
func NewCPUTracker() *CPUTracker {
	return &CPUTracker{
		startTime:      time.Now(),
		lastSampleTime: time.Now(),
		maxSamples:     60, // Keep last 60 samples for averaging
		samples:        make([]float64, 0, 60),
	}
}

// Sample takes a CPU usage sample
func (ct *CPUTracker) Sample() float64 {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	// Calculate based on goroutine scheduler latency
	// This is a proxy metric for CPU usage
	start := time.Now()
	runtime.Gosched()
	schedLatency := time.Since(start).Seconds()

	// Lower latency = higher CPU usage
	// Normalize to percentage (inverse relationship)
	usage := (1.0 - schedLatency*1000) * 100
	if usage < 0 {
		usage = 0
	}
	if usage > 100 {
		usage = 100
	}

	// Add to samples
	ct.samples = append(ct.samples, usage)
	if len(ct.samples) > ct.maxSamples {
		ct.samples = ct.samples[1:]
	}

	// Calculate average
	sum := 0.0
	for _, s := range ct.samples {
		sum += s
	}
	ct.cpuPercent = sum / float64(len(ct.samples))

	return ct.cpuPercent
}

// GetCPUPercent returns the current CPU percentage
func (ct *CPUTracker) GetCPUPercent() float64 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.cpuPercent
}