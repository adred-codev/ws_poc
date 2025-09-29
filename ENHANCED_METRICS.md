# Enhanced Go WebSocket Server Metrics

This document describes the enhanced metrics system implemented for the Go WebSocket server to provide accurate detection of memory usage, CPU usage, and connection tracking.

## Overview

The enhanced metrics system provides three levels of accuracy:

1. **Memory Usage**: Highly accurate using Go's `runtime.MemStats`
2. **CPU Usage**: Estimated using goroutine scheduler latency and system sampling
3. **Connection Tracking**: Precise tracking of individual WebSocket connections

## Architecture

### Core Components

1. **SystemMetrics** (`internal/metrics/system.go`)
   - Collects accurate memory statistics
   - Estimates CPU usage using scheduler latency
   - Provides comprehensive system information

2. **ConnectionTracker** (`internal/metrics/connections.go`)
   - Tracks individual connection lifecycles
   - Records message and byte statistics per connection
   - Monitors connection duration and activity

3. **EnhancedMetrics** (`internal/metrics/enhanced.go`)
   - Orchestrates all metric collection
   - Provides unified API for metrics access
   - Integrates with existing Prometheus metrics

## API Endpoints

### `/stats` (Enhanced Legacy Endpoint)
Returns simplified metrics compatible with the React client:

```json
{
  "connections": {
    "active": 5
  },
  "system": {
    "memory": {
      "heap_alloc": 8388608
    },
    "goroutines": 25
  },
  "nats": {...},
  "hub": {...},
  "uptime_seconds": 120.5
}
```

### `/metrics/enhanced` (Comprehensive Metrics)
Returns detailed metrics for monitoring and analysis:

```json
{
  "timestamp": 1703123456,
  "uptime_seconds": 120.5,
  "last_update": 1703123456,
  "connections": {
    "active": 5,
    "total": 15,
    "peak": 8,
    "messages_sent_total": 1250,
    "messages_recv_total": 340,
    "bytes_sent_total": 125000,
    "bytes_recv_total": 34000,
    "avg_duration_sec": 45.2,
    "connections": [
      {
        "id": "client_abc123",
        "remote_addr": "192.168.1.100:54321",
        "duration_sec": 67.3,
        "messages_sent": 45,
        "messages_recv": 12,
        "bytes_sent": 4500,
        "bytes_recv": 1200,
        "idle_sec": 2.1
      }
    ]
  },
  "system": {
    "memory": {
      "heap_alloc_mb": 8.0,
      "heap_sys_mb": 12.5,
      "heap_idle_mb": 4.5,
      "heap_inuse_mb": 8.0,
      "heap_released_mb": 0.0,
      "stack_inuse_mb": 0.5,
      "sys_total_mb": 16.2,
      "gc_count": 5,
      "gc_cpu_percent": 0.05,
      "goroutines": 25
    },
    "cpu": {
      "percent": 12.5,
      "cores": 8
    }
  },
  "performance": {
    "memory_mb": 8.0,
    "cpu_percent": 12.5,
    "goroutines": 25,
    "active_conns": 5
  }
}
```

### `/metrics/system` (System-Only Metrics)
Returns focused system resource metrics:

```json
{
  "timestamp": 1703123456,
  "system": {
    "memory": {...},
    "cpu": {...}
  },
  "performance": {
    "memory_mb": 8.0,
    "cpu_percent": 12.5,
    "goroutines": 25,
    "active_conns": 5
  }
}
```

## Metric Types

### Memory Metrics
- **heap_alloc_mb**: Currently allocated heap memory
- **heap_sys_mb**: Total heap memory obtained from OS
- **heap_idle_mb**: Idle heap memory
- **heap_inuse_mb**: In-use heap memory
- **heap_released_mb**: Memory released to OS
- **stack_inuse_mb**: Stack memory in use
- **sys_total_mb**: Total memory obtained from OS
- **gc_count**: Number of garbage collections
- **gc_cpu_percent**: CPU time spent in GC

### CPU Metrics
- **percent**: Estimated CPU usage percentage
- **cores**: Number of CPU cores available

*Note: CPU metrics use goroutine scheduler latency as a proxy. For production deployments requiring high CPU accuracy, consider integrating `github.com/shirou/gopsutil/v3/cpu`.*

### Connection Metrics
- **active**: Current active connections
- **total**: Total connections since server start
- **peak**: Peak concurrent connections
- **messages_sent_total**: Total messages sent to clients
- **messages_recv_total**: Total messages received from clients
- **bytes_sent_total**: Total bytes sent
- **bytes_recv_total**: Total bytes received
- **avg_duration_sec**: Average connection duration

## Integration with React Client

The React client automatically uses the enhanced `/stats` endpoint. The metrics tab will now display more accurate data:

1. **Memory Usage**: Real heap allocation from Go runtime
2. **CPU Usage**: Estimated based on scheduler activity
3. **Connection Count**: Precisely tracked active connections

## Performance Impact

- **Memory Overhead**: ~1KB per active connection for tracking
- **CPU Overhead**: <0.1% for metrics collection every 5 seconds
- **Network Overhead**: Minimal increase in endpoint response size

## Configuration

The enhanced metrics system is automatically enabled and requires no additional configuration. It integrates seamlessly with the existing server architecture.

## Production Considerations

### For High-Accuracy CPU Monitoring
Consider integrating a system monitoring library:

```go
import "github.com/shirou/gopsutil/v3/cpu"

// In updateCPUMetrics():
cpuPercent, err := cpu.Percent(time.Second, false)
if err == nil && len(cpuPercent) > 0 {
    em.cpuPercent = cpuPercent[0]
}
```

### For Memory Optimization
Monitor the `connections` array size in the enhanced endpoint. For servers with thousands of connections, consider:

1. Pagination for connection details
2. Summary-only mode
3. Periodic cleanup of stale connection data

### For High-Frequency Updates
The default 5-second update interval balances accuracy with performance. For real-time monitoring:

```go
// Reduce update interval to 1 second
em.updateInterval = 1 * time.Second
```

## Troubleshooting

### High Memory Usage
Check the `heap_alloc_mb` vs `heap_sys_mb` ratio. A large difference indicates memory fragmentation.

### Inaccurate CPU Readings
CPU estimation may be less accurate under very low or very high load. Consider system-level monitoring for critical applications.

### Missing Connection Data
Ensure WebSocket connections are properly registered/unregistered with the enhanced metrics system.

## Testing

To test the enhanced metrics:

1. Start the Go server: `go run cmd/main.go`
2. Access endpoints:
   - http://localhost:3002/stats
   - http://localhost:3002/metrics/enhanced
   - http://localhost:3002/metrics/system
3. Connect WebSocket clients and observe connection tracking
4. Monitor memory and CPU metrics during load

The React client will automatically display the enhanced metrics in its dashboard.