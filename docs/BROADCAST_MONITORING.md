# Broadcast Monitoring Implementation

## Overview

This document describes the monitoring and alerting infrastructure for tracking dropped broadcasts and worker pool health in the WebSocket server.

## Problem Context

During load testing at 15K connections:
- **22,816 dropped broadcasts** over 73 minutes (24% drop rate)
- **6 drops/second** sustained rate
- **0 client disconnections** (excellent connection stability)
- **NATS redelivery** masks drops (0-30 second delay)

This revealed a critical gap: operators had no visibility into broadcast drops until users complained about message delays.

## Implementation

### 1. Metrics Added to Code

**File: `src/worker_pool.go`**

New methods added:
```go
// GetQueueDepth returns current tasks waiting in queue
func (wp *WorkerPool) GetQueueDepth() int

// GetQueueCapacity returns maximum queue size
func (wp *WorkerPool) GetQueueCapacity() int
```

**File: `src/metrics.go`**

New Prometheus metrics:
```
ws_worker_queue_depth              - Current tasks in queue
ws_worker_queue_capacity           - Maximum queue size
ws_worker_queue_utilization_percent - Queue fullness (0-100%)
```

Existing metrics now monitored:
```
ws_dropped_broadcasts_total - Total broadcasts dropped (already existed)
```

### 2. Grafana Dashboard Panels

**Panel 1: Message Delivery Health (Stat Panel)**
- Location: Row 4 (y=32)
- Metrics displayed:
  - Drops per Minute
  - Broadcast Drop Rate %
  - Queue Utilization %
  - Queue Depth (tasks)
- Color coding:
  - 🟢 Green (< 5%): Healthy
  - 🟡 Yellow (5-15%): Approaching capacity
  - 🟠 Orange (15-25%): High load
  - 🔴 Red (> 25%): Critical

**Panel 2: Broadcast Drops & Queue Depth Over Time (Time Series)**
- Location: Row 5 (y=44)
- Tracks:
  - Drop rate trend
  - Queue saturation correlation
  - Historical patterns
- Legend shows: mean, max, last values

### 3. Prometheus Alert Rules

**File: `isolated/backend/alert-rules.yml`**

#### Warning Alerts:

**HighBroadcastDropRate**
- Trigger: > 1 drop/sec for 5 minutes
- Severity: warning
- Action: Monitor and prepare to scale

**WorkerQueueNearCapacity**
- Trigger: Queue > 80% full for 3 minutes
- Severity: warning
- Action: Leading indicator - scale soon

**HighBroadcastDropPercentage**
- Trigger: > 10% of broadcasts dropped for 5 minutes
- Severity: warning
- Action: Sustained capacity issue

#### Critical Alerts:

**CriticalBroadcastDropRate**
- Trigger: > 10 drops/sec for 2 minutes
- Severity: critical
- Action: Immediate scaling required

**WorkerQueueSaturated**
- Trigger: Queue > 95% full for 1 minute
- Severity: critical
- Action: Emergency - drops imminent

## Alert Response Playbook

### Warning: HighBroadcastDropRate (> 1 drop/sec)

**Symptoms:**
- 1-5 drops/second sustained
- Users may report slight delays
- Queue utilization 50-80%

**Actions:**
1. Check Grafana dashboard for queue utilization trend
2. Verify NATS redelivery is working (messages eventually arrive)
3. Plan CPU core scaling if trend continues
4. Monitor for 15-30 minutes before action

**Acceptable if:**
- Drop rate < 5%
- Users not complaining
- NATS redelivery keeping up

### Critical: CriticalBroadcastDropRate (> 10 drops/sec)

**Symptoms:**
- 10+ drops/second
- Queue at 95-100% capacity
- Users reporting 10-30 second delays
- Messages backing up

**Immediate Actions:**
1. **Scale CPU cores** (primary fix):
   ```yaml
   # docker-compose.yml
   cpus: "2.0"  # or "3.0"
   ```
   Deploy immediately: `task deploy:ws-go`

2. **OR reduce connections** (temporary):
   ```bash
   # .env.production
   WS_MAX_CONNECTIONS=12000  # down from 15000
   ```

3. **Check system health:**
   - CPU throttling?
   - Memory pressure?
   - NATS connection stable?

**Do NOT:**
- Increase worker pool size (makes problem worse)
- Restart server without fixing root cause

### Critical: WorkerQueueSaturated (> 95%)

**Symptoms:**
- Queue almost full
- Drops imminent or occurring
- System on edge of collapse

**Immediate Actions:**
1. Check if already dropping (ws_dropped_broadcasts_total increasing)
2. If not dropping yet: Scale CPU NOW (5-10 minute window)
3. If already dropping: Follow CriticalBroadcastDropRate playbook

## Understanding NATS Redelivery

**How it works:**
```
1. Worker pool queue full
2. Submit() drops task (no ACK sent to NATS)
3. NATS waits 30 seconds (JS_CONSUMER_ACK_WAIT)
4. NATS redelivers message
5. Workers process redelivered message
6. Clients receive message (delayed)
```

**Why drops aren't "catastrophic":**
- Messages not lost, just delayed 0-30 seconds
- Better than client disconnections (complete data loss)
- Acceptable for non-HFT trading platforms

**When drops ARE catastrophic:**
- High-frequency trading (30 sec = eternity)
- Real-time auction systems
- Live sports betting
- Time-sensitive alerts

## Design Trade-off: 192 Workers

**Why NOT 512 workers:**
```
512 workers = 2ms broadcasts = BURST traffic
              ↓
writePump can't drain fast enough
              ↓
Client send buffers fill
              ↓
969 slow client disconnections (23% failure)
```

**Why 192 workers:**
```
192 workers = 20ms broadcasts = SMOOTH traffic
              ↓
writePump keeps up with natural pacing
              ↓
0 slow client disconnections ✅
              ↓
BUT: 24% worker drops (NATS redelivers)
```

**Trade-off chosen:**
- Delayed messages (NATS redelivery) > Disconnections (data loss)
- Connection stability > Broadcast throughput
- User experience > Perfect metrics

## Capacity Planning

### Current Capacity (15K connections @ 1 CPU core):
```
Theoretical: 50 broadcasts/sec (1000ms ÷ 20ms)
Actual: ~19-21 broadcasts/sec (burst spikes, GC pauses, lock overhead)
Publisher: 25 broadcasts/sec
Result: 24% drop rate
```

### Scaling Options:

**Option A: Accept Current (24% drops)**
- Pros: Stable connections, NATS redelivery works
- Cons: 0-30 sec message delays
- Best for: Non-HFT trading, general real-time apps

**Option B: Scale to 2-3 CPU Cores**
- Pros: Eliminates drops, maintains 15K connections
- Cons: Requires lock-free data structures first
- Best for: After implementing SubscriptionIndex sharding

**Option C: Reduce to 12K Connections**
- Pros: Eliminates drops, keeps 1 CPU core
- Cons: 20% less capacity
- Best for: Conservative approach

## Files Modified

### Code Changes:
- `src/worker_pool.go` - Added queue depth methods
- `src/metrics.go` - Added queue metrics collection

### Configuration:
- `isolated/backend/prometheus.yml` - Load alert rules
- `isolated/backend/docker-compose.yml` - Mount alert-rules.yml
- `isolated/backend/alert-rules.yml` - NEW (alert definitions)

### Monitoring:
- `grafana/provisioning/dashboards/websocket.json` - Added 2 panels

## Testing After Deployment

1. **Verify metrics available:**
   ```bash
   curl http://ws-go:3004/metrics | grep ws_worker_queue
   ```
   Should show: depth, capacity, utilization

2. **Check Grafana panels:**
   - Navigate to WebSocket Dashboard
   - Scroll to "Message Delivery Health" panel
   - Verify metrics populating

3. **Test alerts (optional):**
   ```bash
   # Trigger warning alert by overloading
   # Verify alert fires in Prometheus: http://localhost:9091/alerts
   ```

## Future Improvements

1. **Lock-free SubscriptionIndex** - Enable multi-core scaling
2. **Per-client rate limiting** - Protect against abusive clients
3. **Message latency histogram** - Measure actual user-perceived delays
4. **Auto-scaling triggers** - Kubernetes HPA based on queue utilization
5. **NATS redelivery counter** - Track actual redelivery rate

## Related Documentation

- `.env.production` - Worker pool configuration and rationale
- `docs/production/IMPLEMENTATION_PLAN.md` - Original capacity planning
- `src/worker_pool.go` - Worker pool implementation

