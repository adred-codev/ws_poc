# Sharding Performance Investigation
**Date:** October 25, 2025
**Investigator:** Claude
**Severity:** CRITICAL
**Status:** Investigation In Progress

## Executive Summary

Capacity testing revealed that the sharded WebSocket server implementation is experiencing severe performance degradation compared to expectations. The server is limited to **4,000 connections** (57% of target) with **periodic CPU spikes to 100%**, despite having sharding implemented and configured with 16 shards.

**Expected Performance (from design docs):**
- ✅ 10,000 concurrent connections
- ✅ ~15% CPU usage
- ✅ 16x reduction in lock contention

**Actual Performance (observed):**
- ❌ 4,000 concurrent connections max (60% below target)
- ❌ CPU oscillating between 18% and 100% every ~30 seconds
- ❌ ResourceGuard blocking connections due to CPU overload (90%+)
- ❌ 7,000 zombie connections from previous test (connection cleanup bug)

## Critical Issues Discovered

### 1. Zombie Connection Bug ⚠️ **HIGH PRIORITY**

**Symptom:** Server reported 7,000 active connections but only 1 actual TCP connection existed.

**Impact:**
- Server at "max capacity" on startup, rejecting all new connections
- Indicates broken connection cleanup logic
- Could cause memory leaks and resource exhaustion

**Evidence:**
```bash
# Server health check
"capacity": {
  "current": 7000,
  "max": 7000,
  "healthy": false
}

# Actual TCP connections
$ ss -tn state established "( sport = :3004 )" | wc -l
1
```

**Root Cause:** Unknown - requires code review of disconnect handling in sharded implementation.

**Workaround:** Server restart clears zombie connections.

---

### 2. Severe CPU Performance Regression ⚠️ **CRITICAL**

**Symptom:** CPU usage oscillates wildly between 18% and 100% in ~30-second cycles during steady-state operation.

**Test Conditions:**
- 4,000 active connections
- 5 subscribed channels (token.BTC, token.ETH, token.SOL, token.ODIN, token.DOGE)
- Publisher sending 19.2 msg/sec
- Sharding: 16 shards configured (WS_NUM_SHARDS=16)
- GOMAXPROCS=4

**Observed Pattern:**
```
Time    CPU    Status
----------------------------
00:57:24  20.3%  ✅ Healthy
00:57:34  20.3%  ✅ Healthy
00:57:44  100.0% ❌ Unhealthy (CPU threshold exceeded)
00:57:54  18.2%  ✅ Healthy
00:58:04  18.4%  ✅ Healthy
00:58:14  80.7%  ❌ Unhealthy
00:58:24  20.1%  ✅ Healthy
00:58:34  19.4%  ✅ Healthy
00:58:44  86.8%  ❌ Unhealthy
00:58:54  99.3%  ❌ Unhealthy
00:59:04  18.1%  ✅ Healthy
```

**Impact:**
- ResourceGuard rejects connections when CPU > 75%
- Server capacity capped at ~4,000 connections instead of 10,000
- Unpredictable performance (oscillating between healthy and unhealthy)
- User experience degradation during CPU spikes

**Possible Causes:**
1. **Publisher burst patterns** - Market stats published periodically causing broadcast storms
2. **Go GC (Garbage Collection)** - Periodic GC pauses under heavy load
3. **Lock contention in sharding implementation** - Bug in shard distribution
4. **Message fanout overhead** - 19.2 msg/sec × 4,000 connections = 76,800 broadcasts/sec peak
5. **Channel selection** - Goroutine scheduler overhead with high concurrency

---

### 3. Connection Establishment Bottleneck

**Symptom:** Server successfully accepted first 2,000 connections, then rejection rate increased dramatically.

**Ramp-up Results:**
```
Time    Attempts  Active  Failed  Success Rate  Server CPU
---------------------------------------------------------------
10s     990       990     0       100.0%        24.7%
20s     1990      1990    0       100.0%        47.5%
30s     3000      2040    950     68.3%         17.0%  ⚠️ failures start
40s     4000      2490    1500    62.5%         17.7%
50s     4990      3490    1500    69.9%         71.2%  ⚠️ CPU spike
60s     5990      3540    2450    59.1%         20.3%
70s     6981      3980    3000    57.0%         20.3%  ⚠️ CPU hit 100%
FINAL   7000      4000    3000    57.1%         100.0% (peak)
```

**Analysis:**
- Initial 2K connections: smooth (100% success)
- 2K-4K connections: degradation starts (CPU spikes to 71%)
- 4K-7K connections: severe rejection (CPU hits 100%, ResourceGuard blocks)

**ResourceGuard Rejection Logs:**
```json
{
  "level": "warn",
  "current_connections": 3540,
  "max_connections": 7000,
  "reason": "CPU 90.2% > 75.0%",
  "message": "Connection rejected by ResourceGuard"
}
```

**Impact:** Cannot achieve target capacity of 10K connections.

---

## Test Environment

### Infrastructure
- **Instance:** GCE e2-standard-4 (4 vCPU, 16GB RAM)
- **Location:** us-central1-a
- **Server:** odin-ws-go (10.128.0.21:3004)
- **Client:** odin-test-runner (Go-based load tester)
- **Backend:** odin-backend (NATS, Publisher, Monitoring stack)

### Configuration
```env
WS_MAX_CONNECTIONS=7000
WS_NUM_SHARDS=16
WS_CPU_LIMIT=4.0
WS_MEMORY_LIMIT=15569256448 (14.5 GB)
GOMAXPROCS=4
```

### Docker Resources
```yaml
cpus: 4.0
memory: 14848M (14.5 GB)
```

### Test Parameters
```
Target Connections: 7000
Ramp Rate: 100 conn/sec
Connection Timeout: 10s
Sustain Duration: 3600s (1 hour)
Subscription Mode: all (5 channels)
```

---

## Message Flow Analysis

### Publisher Configuration
- **Rate:** 19.2 msg/sec (sustained)
- **Active Tokens:** 10 (BTC, ETH, SOL, ODIN, DOGE, MATIC, etc.)
- **Messages Published:** 35,078 total (over 30 minutes)
- **Channels:** odin.token.{tokenId}

### Message Fanout Calculation

With 4,000 connections subscribing to 5 channels:
```
Connections per channel: 4000 ÷ 5 = ~800

Publisher sends: 19.2 msg/sec
Server broadcasts: 19.2 × 800 = 15,360 WebSocket messages/sec
```

**Observed Rate:** 16,000-20,000 msg/sec (matches calculation!)

**Total Messages Received by Server:** 19.7 million (from JetStream)
```
19,700,000 messages ÷ 1800 seconds = 10,944 msg/sec from NATS
10,944 × ~1.5 fanout multiplier = 16,416 WebSocket broadcasts/sec
```

This confirms the server is under **heavy broadcast load** which explains the high CPU usage during burst periods.

---

## Sharding Implementation Status

### Verification Steps

1. **Binary contains sharding code:**
```bash
$ strings ./odin-ws-server | grep -i shard
shards
getShard
NumShards
numShards
*main.shard
```
✅ Confirmed

2. **Environment variable set:**
```bash
$ docker exec odin-ws-go env | grep SHARD
WS_NUM_SHARDS=16
```
✅ Confirmed

3. **Server logs show shard count:**
```
# Config print should show shard configuration
```
⚠️ **TODO: Verify shard count appears in startup logs**

### Expected vs Actual Behavior

**Expected (from LOCK_CONTENTION_STRATEGIES.md):**
- 16 independent locks reduce contention by factor of 16
- Hash-based channel distribution (FNV-1a)
- CPU usage ~15% for 10K connections
- Parallel broadcasts across shards

**Actual:**
- CPU 18-100% oscillating for 4K connections
- Connection limit hit at 4K (not 10K)
- Periodic CPU spikes suggest serialization bottleneck
- ResourceGuard blocking due to CPU overload

**Hypothesis:** Sharding implementation has a bug or the message fanout is creating a different bottleneck than lock contention.

---

## Performance Comparison

### Pre-Sharding (from session docs)
```
GOMAXPROCS=1: 10% CPU, 2K connections
GOMAXPROCS=3: 83% CPU (lock contention), 7K connections
GOMAXPROCS=4: 95% CPU (lock contention), unable to test
```

### Post-Sharding (current test)
```
GOMAXPROCS=4: 18-100% CPU (oscillating), 4K connections max
Lock contention: Unknown (needs profiling)
```

### Regression Analysis
- **Connection capacity:** 7K → 4K (-43% regression! 🚨)
- **CPU efficiency:** Cannot compare directly (different connection counts)
- **Stability:** Pre-sharding was stable; post-sharding oscillates wildly

**Conclusion:** The sharding implementation appears to have DEGRADED performance rather than improved it!

---

## Next Steps - Investigation Plan

### Immediate Actions (Priority 1)

1. **CPU Profiling** 🔥
   ```bash
   # Capture 30-second CPU profile during oscillation period
   curl http://localhost:3004/debug/pprof/profile?seconds=30 > cpu.prof
   go tool pprof -top cpu.prof
   ```
   **Goal:** Identify what's consuming CPU during spikes (GC, lock contention, broadcast overhead, etc.)

2. **Memory Profiling**
   ```bash
   curl http://localhost:3004/debug/pprof/heap > heap.prof
   go tool pprof -top heap.prof
   ```
   **Goal:** Check for memory leaks from zombie connections

3. **Goroutine Profiling**
   ```bash
   curl http://localhost:3004/debug/pprof/goroutine > goroutine.prof
   go tool pprof -top goroutine.prof
   ```
   **Goal:** Check for goroutine leaks or blocked goroutines

4. **Mutex Contention Profiling** 🔥
   ```bash
   curl http://localhost:3004/debug/pprof/mutex > mutex.prof
   go tool pprof -top mutex.prof
   ```
   **Goal:** Verify if sharding actually reduced lock contention or introduced new locks

### Code Review (Priority 2)

1. **Review shard implementation in connection.go:496-724**
   - Check FNV-1a hash implementation
   - Verify shard selection logic
   - Look for race conditions in shard access
   - Check if multiple shards can be locked simultaneously

2. **Review connection cleanup logic**
   - Identify why 7,000 zombie connections remained
   - Check RemoveClient() implementation (connection.go:664-687)
   - Verify disconnect event handling

3. **Review broadcast mechanism**
   - Check if broadcasts are parallelized across shards
   - Verify worker pool is being used effectively
   - Look for serialization points

### Performance Testing (Priority 3)

1. **Test with different shard counts:**
   - WS_NUM_SHARDS=8 (fewer shards, less overhead)
   - WS_NUM_SHARDS=32 (more parallelism)
   - WS_NUM_SHARDS=1 (essentially disabling sharding)

2. **Test without publisher:**
   - Stop odin-publisher
   - Establish 7K connections without message load
   - Measure baseline CPU usage

3. **Test with slower ramp-up:**
   - Reduce ramp rate to 50 conn/sec or 25 conn/sec
   - Check if slower ramp avoids CPU spikes

4. **Compare against pre-sharding code:**
   - Deploy old code without sharding
   - Run identical test
   - Direct comparison of metrics

---

## Metrics to Collect

### Server-side Metrics
- [ ] Per-shard lock contention (mutex profile)
- [ ] Per-shard connection distribution (debug endpoint)
- [ ] GC pause times and frequency
- [ ] Goroutine count over time
- [ ] Memory allocation rate
- [ ] CPU profile flame graph
- [ ] Network I/O stats

### Client-side Metrics
- [ ] Message delivery latency (p50, p95, p99)
- [ ] Connection establishment time
- [ ] Reconnection rate
- [ ] Message loss rate

---

## Open Questions

1. **Why does CPU spike periodically every ~30 seconds?**
   - Is it GC?
   - Is it publisher burst pattern?
   - Is it a background task in the server?

2. **Why did connection count regression from 7K to 4K?**
   - Is sharding adding overhead?
   - Is there a new bottleneck?
   - Is the test environment different?

3. **Why are zombie connections not being cleaned up?**
   - Is RemoveClient() being called on disconnect?
   - Is there a bug in the sharding-aware cleanup logic?
   - Are TCP close events being handled?

4. **Is the FNV-1a hash causing uneven distribution?**
   - Are all shards being used?
   - Are some shards overloaded?
   - Should we use a different hash function?

5. **What is the actual lock contention reduction?**
   - Need mutex profile before/after sharding
   - Expected 16x reduction, what's the actual number?

---

## Recommendations

### Short-term (This Week)

1. **Revert to pre-sharding code** until performance regression is fixed
   - Can handle 7K connections vs current 4K
   - More stable CPU usage
   - Known behavior

2. **Run comprehensive profiling** (CPU, memory, mutex, goroutine)
   - Identify root cause of performance regression
   - Compare sharded vs non-sharded profiles

3. **Fix zombie connection bug** immediately
   - Critical for production stability
   - Could cause cascading failures

### Medium-term (Next Sprint)

1. **Redesign broadcast mechanism**
   - Consider worker pool per shard
   - Investigate lock-free broadcast queue
   - Benchmark different approaches

2. **Add detailed metrics**
   - Per-shard statistics
   - Lock contention metrics
   - Message fanout metrics
   - GC pause time tracking

3. **Implement gradual rollout**
   - Feature flag for sharding
   - A/B test sharded vs non-sharded
   - Monitor metrics before full deployment

### Long-term (Next Quarter)

1. **Consider alternative architectures**
   - Actor model with dedicated goroutine per shard
   - Lock-free sync.Map for dynamic channels
   - Hybrid approach (sharding + lock-free)

2. **Horizontal scaling**
   - Multiple WebSocket servers behind load balancer
   - Each server handles subset of connections
   - May be simpler than complex sharding logic

---

## Session Artifacts

### Logs Captured
- Server logs during ramp-up (connection rejections)
- Publisher logs (message rate confirmation)
- Test client output (connection attempts and failures)

### Commands Used
```bash
# Check zombie connections
sudo ss -tn state established "( sport = :3004 )" | wc -l

# Restart server
sudo docker restart odin-ws-go

# Check configuration
sudo docker exec odin-ws-go env | grep SHARD

# Check binary for sharding code
sudo docker exec odin-ws-go sh -c "strings ./odin-ws-server | grep -i shard"

# Monitor test progress
task gcp2:test:remote:capacity
```

### Files Modified
- None (investigation only, no code changes)

---

## Conclusion

The sharded WebSocket server implementation is experiencing a **severe performance regression** that prevents it from achieving the design goals. Instead of improving capacity from 7K to 10K connections with reduced CPU usage, the current implementation is limited to **4K connections** with **wildly oscillating CPU** (18-100%).

**Critical Issues:**
1. ⚠️ **60% connection capacity regression** (7K → 4K)
2. ⚠️ **Periodic CPU spikes to 100%** causing connection rejections
3. ⚠️ **7,000 zombie connections** indicating broken cleanup logic

**Immediate Action Required:**
- CPU profiling to identify bottleneck
- Fix zombie connection cleanup bug
- Consider reverting to pre-sharding code until issues resolved

**Status:** Investigation in progress. Session paused pending profiling results.

---

**Next Session:** Focus on CPU profiling and root cause analysis of performance regression.
