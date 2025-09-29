package metrics

import (
	"runtime/metrics"
	"sync"
	"time"
)

// RuntimeMetricsReader provides comprehensive Go runtime metrics using runtime/metrics
type RuntimeMetricsReader struct {
	mu      sync.RWMutex
	samples []metrics.Sample
	values  map[string]interface{}
}

// NewRuntimeMetricsReader creates a new runtime metrics reader
func NewRuntimeMetricsReader() *RuntimeMetricsReader {
	// Get all available metrics descriptions
	descs := metrics.All()

	// Create samples slice with only the metrics we care about
	metricNames := []string{
		// Memory metrics
		"/memory/classes/heap/free:bytes",
		"/memory/classes/heap/released:bytes",
		"/memory/classes/heap/stacks:bytes",
		"/memory/classes/heap/objects:bytes",
		"/memory/classes/total:bytes",

		// GC metrics
		"/gc/cycles/automatic:gc-cycles",
		"/gc/cycles/forced:gc-cycles",
		"/gc/heap/allocs:bytes",
		"/gc/heap/frees:bytes",
		"/gc/heap/goal:bytes",
		"/gc/heap/tiny/allocs:objects",
		"/gc/pauses:seconds",

		// Scheduler metrics
		"/sched/goroutines:goroutines",
		"/sched/latencies:seconds",

		// CPU metrics (Go-related)
		"/cpu/classes/gc/mark/assist:cpu-seconds",
		"/cpu/classes/gc/mark/dedicated:cpu-seconds",
		"/cpu/classes/gc/pause:cpu-seconds",
		"/cpu/classes/gc/total:cpu-seconds",
		"/cpu/classes/idle:cpu-seconds",
		"/cpu/classes/scavenge/assist:cpu-seconds",
		"/cpu/classes/scavenge/background:cpu-seconds",
		"/cpu/classes/scavenge/total:cpu-seconds",
		"/cpu/classes/total:cpu-seconds",
		"/cpu/classes/user:cpu-seconds",
	}

	samples := make([]metrics.Sample, 0, len(metricNames))

	// Only include metrics that actually exist
	for _, name := range metricNames {
		for _, desc := range descs {
			if desc.Name == name {
				samples = append(samples, metrics.Sample{Name: name})
				break
			}
		}
	}

	return &RuntimeMetricsReader{
		samples: samples,
		values:  make(map[string]interface{}),
	}
}

// Update reads the latest runtime metrics
func (rmr *RuntimeMetricsReader) Update() {
	rmr.mu.Lock()
	defer rmr.mu.Unlock()

	// Read all samples
	metrics.Read(rmr.samples)

	// Convert to map for easy access
	rmr.values = make(map[string]interface{})
	for _, sample := range rmr.samples {
		switch sample.Value.Kind() {
		case metrics.KindUint64:
			rmr.values[sample.Name] = sample.Value.Uint64()
		case metrics.KindFloat64:
			rmr.values[sample.Name] = sample.Value.Float64()
		case metrics.KindFloat64Histogram:
			// For histograms, we'll extract useful stats
			hist := sample.Value.Float64Histogram()
			count := uint64(0)

			// Calculate count from buckets
			if hist.Counts != nil {
				for _, count_val := range hist.Counts {
					count += count_val
				}
			}

			rmr.values[sample.Name] = map[string]interface{}{
				"count":   count,
				"buckets": len(hist.Buckets),
			}
		}
	}
}

// GetEnhancedMemoryStats returns detailed memory statistics
func (rmr *RuntimeMetricsReader) GetEnhancedMemoryStats() map[string]interface{} {
	rmr.mu.RLock()
	defer rmr.mu.RUnlock()

	stats := make(map[string]interface{})

	// Convert bytes to MB for readability
	bytesToMB := func(val interface{}) float64 {
		if v, ok := val.(uint64); ok {
			return float64(v) / 1024 / 1024
		}
		return 0
	}

	// Memory classes
	if val, ok := rmr.values["/memory/classes/heap/free:bytes"]; ok {
		stats["heap_free_mb"] = bytesToMB(val)
	}
	if val, ok := rmr.values["/memory/classes/heap/released:bytes"]; ok {
		stats["heap_released_mb"] = bytesToMB(val)
	}
	if val, ok := rmr.values["/memory/classes/heap/stacks:bytes"]; ok {
		stats["heap_stacks_mb"] = bytesToMB(val)
	}
	if val, ok := rmr.values["/memory/classes/heap/objects:bytes"]; ok {
		stats["heap_objects_mb"] = bytesToMB(val)
	}
	if val, ok := rmr.values["/memory/classes/total:bytes"]; ok {
		stats["total_memory_mb"] = bytesToMB(val)
	}

	return stats
}

// GetGCStats returns detailed garbage collection statistics
func (rmr *RuntimeMetricsReader) GetGCStats() map[string]interface{} {
	rmr.mu.RLock()
	defer rmr.mu.RUnlock()

	stats := make(map[string]interface{})

	if val, ok := rmr.values["/gc/cycles/automatic:gc-cycles"]; ok {
		stats["gc_cycles_auto"] = val
	}
	if val, ok := rmr.values["/gc/cycles/forced:gc-cycles"]; ok {
		stats["gc_cycles_forced"] = val
	}
	if val, ok := rmr.values["/gc/heap/allocs:bytes"]; ok {
		if v, ok := val.(uint64); ok {
			stats["gc_heap_allocs_mb"] = float64(v) / 1024 / 1024
		}
	}
	if val, ok := rmr.values["/gc/heap/frees:bytes"]; ok {
		if v, ok := val.(uint64); ok {
			stats["gc_heap_frees_mb"] = float64(v) / 1024 / 1024
		}
	}
	if val, ok := rmr.values["/gc/heap/goal:bytes"]; ok {
		if v, ok := val.(uint64); ok {
			stats["gc_heap_goal_mb"] = float64(v) / 1024 / 1024
		}
	}
	if val, ok := rmr.values["/gc/heap/tiny/allocs:objects"]; ok {
		stats["gc_tiny_allocs"] = val
	}

	// GC pause times (histogram)
	if val, ok := rmr.values["/gc/pauses:seconds"]; ok {
		stats["gc_pauses"] = val
	}

	return stats
}

// GetSchedulerStats returns goroutine and scheduler statistics
func (rmr *RuntimeMetricsReader) GetSchedulerStats() map[string]interface{} {
	rmr.mu.RLock()
	defer rmr.mu.RUnlock()

	stats := make(map[string]interface{})

	if val, ok := rmr.values["/sched/goroutines:goroutines"]; ok {
		stats["goroutines"] = val
	}

	// Scheduler latencies (histogram)
	if val, ok := rmr.values["/sched/latencies:seconds"]; ok {
		stats["scheduler_latencies"] = val
	}

	return stats
}

// GetCPUStats returns CPU-related statistics from Go runtime
func (rmr *RuntimeMetricsReader) GetCPUStats() map[string]interface{} {
	rmr.mu.RLock()
	defer rmr.mu.RUnlock()

	stats := make(map[string]interface{})

	// CPU time breakdown
	if val, ok := rmr.values["/cpu/classes/gc/total:cpu-seconds"]; ok {
		stats["cpu_gc_seconds"] = val
	}
	if val, ok := rmr.values["/cpu/classes/idle:cpu-seconds"]; ok {
		stats["cpu_idle_seconds"] = val
	}
	if val, ok := rmr.values["/cpu/classes/scavenge/total:cpu-seconds"]; ok {
		stats["cpu_scavenge_seconds"] = val
	}
	if val, ok := rmr.values["/cpu/classes/total:cpu-seconds"]; ok {
		stats["cpu_total_seconds"] = val
	}
	if val, ok := rmr.values["/cpu/classes/user:cpu-seconds"]; ok {
		stats["cpu_user_seconds"] = val
	}

	return stats
}

// GetAllStats returns all runtime metrics in a structured format
func (rmr *RuntimeMetricsReader) GetAllStats() map[string]interface{} {
	return map[string]interface{}{
		"memory":    rmr.GetEnhancedMemoryStats(),
		"gc":        rmr.GetGCStats(),
		"scheduler": rmr.GetSchedulerStats(),
		"cpu":       rmr.GetCPUStats(),
		"timestamp": time.Now().Unix(),
	}
}

// GetRawValue returns a raw metric value by name
func (rmr *RuntimeMetricsReader) GetRawValue(name string) interface{} {
	rmr.mu.RLock()
	defer rmr.mu.RUnlock()

	return rmr.values[name]
}