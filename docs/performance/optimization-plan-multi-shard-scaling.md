# Multi-Shard Scaling Optimization Plan

**Date**: 2025-11-13
**Context**: 7-shard configuration on e2-highcpu-8 underperforms 3-shard configuration on e2-standard-4
**Goal**: Achieve linear scaling with increased shard count

---

## 📊 Problem Statement

### Current Performance

| Configuration | Connections | Success Rate | Per-Core Efficiency |
|--------------|-------------|--------------|-------------------|
| **3 shards (e2-standard-4)** | 5,180 / 12,000 | 43.2% | 1,762 conn/core |
| **7 shards (e2-highcpu-8)** | 4,754 / 12,000 | 39.6% | 701 conn/core |
| **Result** | -426 (-8.2%) | -3.6pp | -60% efficiency ❌ |

### Root Cause

Infrastructure overhead scales linearly with shard count, but performance benefits don't:

**Per-Shard Overhead (~20 goroutines each):**
- Kafka consumer - partition coordination, rebalancing, message fetching
- LoadBalancer proxy - WebSocket tunneling, buffer copying
- BroadcastBus - Message fan-out to N shards
- CPU admission control - Repeated cgroup reads (7× vs 3×)
- Health checks - Per-shard monitoring
- Infrastructure goroutines - Metrics, logging, coordination

**Overhead Scaling Math:**
- 3 shards: 60 infrastructure goroutines + 5,180 connections = 10,420 total
- 7 shards: 140 infrastructure goroutines + 4,754 connections = 9,648 total
- **Result**: 133% more overhead, but 8.2% fewer connections handled

---

## 🎯 Optimization Strategy

### Three-Phase Approach

1. **Phase 1: Quick Wins** (Week 1-2) - Low effort, immediate impact
2. **Phase 2: High-ROI Medium Changes** (Week 3-6) - Moderate effort, high impact
3. **Phase 3: Architectural Changes** (Week 7+) - High effort, transformative impact (only if needed)

---

## 🚀 Phase 1: Quick Wins (Week 1-2)

**Target**: +850 connections → 5,500-5,800 total
**Total Effort**: 8-9 hours (1-2 days)

### Opt #1: Centralized CPU Admission Control ⚡

**Problem**: Each shard does redundant cgroup reads (7× vs 3×)

**Solution**:
- Move CPU checking to LoadBalancer
- LoadBalancer shares admission state via atomic flag
- Shards check atomic flag instead of reading cgroups

**Implementation**:

```go
// ws/internal/loadbalancer/loadbalancer.go
type LoadBalancer struct {
    cpuOverloaded atomic.Bool  // Shared state
    // ...
}

func (lb *LoadBalancer) monitorCPU() {
    ticker := time.NewTicker(100 * time.Millisecond)
    for range ticker.C {
        cpu := readCPUFromCgroup()
        lb.cpuOverloaded.Store(cpu > 90.0)
    }
}

// ws/internal/shard/shard.go
func (s *Shard) shouldAdmitConnection() bool {
    if s.lb.cpuOverloaded.Load() {
        return false
    }
    // ... rest of admission logic
}
```

**Files to modify:**
- `ws/internal/loadbalancer/loadbalancer.go` - Add shared CPU state
- `ws/internal/shard/shard.go` - Read atomic flag instead of cgroup

**Impact**: LOW (reduces 7 cgroup reads to 1 per admission cycle)
**Effort**: LOW (2-3 hours)
**Estimated gain**: +150-200 connections

---

### Opt #2: Sampled CPU Monitoring ⚡

**Problem**: CPU checked on every connection attempt

**Solution**:
- Sample CPU every N connections (e.g., N=100)
- Cache CPU state for 100-200ms
- Use cached value for admission decisions

**Implementation**:

```go
// ws/internal/shared/limits/resource_guard.go
type ResourceGuard struct {
    cpuCache      atomic.Value  // Cached CPU reading
    cpuCacheTime  atomic.Int64  // Unix nano
    cacheTTL      time.Duration // 100ms
}

func (rg *ResourceGuard) GetCPU() float64 {
    now := time.Now().UnixNano()
    cacheTime := rg.cpuCacheTime.Load()

    // Return cached value if fresh
    if now-cacheTime < int64(rg.cacheTTL) {
        return rg.cpuCache.Load().(float64)
    }

    // Refresh cache
    cpu := readCPUFromCgroup()
    rg.cpuCache.Store(cpu)
    rg.cpuCacheTime.Store(now)
    return cpu
}
```

**Files to modify:**
- `ws/internal/shared/limits/resource_guard.go`

**Impact**: LOW-MEDIUM (reduces cgroup syscalls by 99%)
**Effort**: LOW (1 hour)
**Estimated gain**: +100-150 connections

---

### Opt #3: Kafka Message Batching ⚡

**Problem**: Processing overhead per message

**Solution**:
- Batch-fetch messages from Kafka (fetch.min.bytes, fetch.max.wait.ms)
- Process multiple messages in one goroutine cycle
- Reduces context switching

**Implementation**:

```go
// ws/internal/kafka/consumer.go
func (c *Consumer) Start() {
    for {
        // Fetch batch of messages (up to 100)
        messages := c.reader.FetchMessages(ctx, 100, 50*time.Millisecond)

        // Process in batch (single goroutine iteration)
        for _, msg := range messages {
            c.handler.Handle(msg)
        }

        // Commit offset once per batch
        c.reader.CommitMessages(ctx, messages...)
    }
}
```

**Files to modify:**
- `ws/internal/kafka/consumer.go`

**Impact**: MEDIUM (20-30% Kafka overhead reduction)
**Effort**: LOW (2 hours)
**Estimated gain**: +300-400 connections

---

### Opt #4: Broadcast Message Batching ⚡

**Problem**: Each broadcast is separate fan-out operation

**Solution**:
- Buffer broadcasts for 10-50ms window
- Send batched messages to all shards at once
- Reduces per-message fan-out overhead

**Implementation**:

```go
// ws/internal/broadcast/bus.go
type BroadcastBus struct {
    batchBuffer []Message
    batchTimer  *time.Timer
    mu          sync.Mutex
}

func (bb *BroadcastBus) Broadcast(msg Message) {
    bb.mu.Lock()
    bb.batchBuffer = append(bb.batchBuffer, msg)

    // Flush if buffer full or timer expires
    if len(bb.batchBuffer) >= 10 {
        bb.flushBatch()
    } else if bb.batchTimer == nil {
        bb.batchTimer = time.AfterFunc(10*time.Millisecond, bb.flushBatch)
    }
    bb.mu.Unlock()
}

func (bb *BroadcastBus) flushBatch() {
    batch := bb.batchBuffer
    bb.batchBuffer = nil
    bb.batchTimer = nil

    // Send batch to all shards at once
    for _, shard := range bb.shards {
        shard.SendBatch(batch)  // Single channel send vs N sends
    }
}
```

**Files to modify:**
- `ws/internal/broadcast/bus.go`

**Impact**: MEDIUM (reduces broadcast overhead by 50-80%)
**Effort**: LOW (3 hours)
**Estimated gain**: +200-300 connections

---

**Phase 1 Summary:**

| Optimization | Effort | Gain | Priority |
|-------------|--------|------|----------|
| Centralized CPU check | 2-3h | +150-200 | P1 |
| Sampled CPU monitoring | 1h | +100-150 | P1 |
| Kafka batching | 2h | +300-400 | P1 |
| Broadcast batching | 3h | +200-300 | P1 |
| **TOTAL** | **8-9h** | **+750-1,050** | - |

**Expected Result**: 5,500-5,800 connections (16-22% improvement, surpasses 3-shard baseline)

---

## 🔥 Phase 2: High-Impact Changes (Week 3-6)

**Target**: +1,600 connections → 7,100-8,200 total
**Total Effort**: 2-3 weeks

### Opt #5: Shared Kafka Consumer Pool 🔥 (HIGHEST PRIORITY)

**Problem**: 7 independent Kafka consumers = 7× overhead

**Solution**:
- Use 2-3 dedicated Kafka consumer goroutines (not per-shard)
- Consumers distribute messages to shards via buffered channels
- Eliminates consumer group coordination per-shard

**Architecture**:

```
OLD: [Shard0+Consumer] [Shard1+Consumer] ... [Shard6+Consumer]
NEW: [Consumer Pool] → channels → [Shard0] [Shard1] ... [Shard6]

[Consumer 0] ─┐
[Consumer 1] ─┼→ [Router] → buffered channels → [Shard 0-6]
[Consumer 2] ─┘
```

**Implementation**:

```go
// ws/internal/kafka/pool.go (NEW FILE)
type ConsumerPool struct {
    consumers []*Consumer
    router    *MessageRouter
}

func (cp *ConsumerPool) Start(numConsumers int, shards []*Shard) {
    for i := 0; i < numConsumers; i++ {
        go cp.consumeLoop(i)
    }
}

func (cp *ConsumerPool) consumeLoop(id int) {
    for msg := range cp.consumers[id].Messages() {
        // Determine target shard(s)
        shards := cp.router.Route(msg)

        // Send to shard channels
        for _, shard := range shards {
            shard.msgChan <- msg
        }
    }
}
```

**Files to create/modify:**
- `ws/internal/kafka/pool.go` (NEW) - Consumer pool with work distribution
- `ws/internal/shard/shard.go` - Remove per-shard consumers
- `ws/internal/multi/multi.go` - Initialize shared pool

**Impact**: HIGH (70-85% Kafka overhead reduction)
**Effort**: MEDIUM-HIGH (1 week)
**Estimated gain**: +800-1,200 connections

---

### Opt #6: Smart Message Routing 🔥

**Problem**: All messages broadcast to all shards (even targeted ones)

**Solution**:
- Build routing table: connection_id → shard_id
- Point-to-point messages routed only to target shard
- Only true broadcasts fan-out to all shards

**Implementation**:

```go
// ws/internal/broadcast/router.go (NEW FILE)
type MessageRouter struct {
    // connID → shardID mapping
    connMap sync.Map
}

func (mr *MessageRouter) Route(msg Message) []*Shard {
    if msg.IsBroadcast {
        return allShards  // True broadcast
    }

    // Point-to-point: route to specific shard
    if shardID, ok := mr.connMap.Load(msg.ConnectionID); ok {
        return []*Shard{shards[shardID]}
    }

    return allShards  // Fallback
}

// ws/internal/loadbalancer/loadbalancer.go
func (lb *LoadBalancer) TrackConnection(connID string, shardID int) {
    lb.router.connMap.Store(connID, shardID)
}
```

**Files to modify:**
- `ws/internal/broadcast/router.go` (NEW)
- `ws/internal/loadbalancer/loadbalancer.go` - Track connection → shard mapping

**Impact**: MEDIUM-HIGH (30-50% broadcast reduction)
**Effort**: MEDIUM (3-4 days)
**Estimated gain**: +400-600 connections

---

### Opt #7: Unix Domain Sockets for LB-Shard Communication 🔥

**Problem**: TCP between LoadBalancer and shards adds network stack overhead

**Solution**:
- Replace TCP with Unix domain sockets (UDS)
- Eliminates TCP/IP stack, reduces buffer copies
- Only works for co-located processes (same container/host)

**Implementation**:

```go
// ws/internal/loadbalancer/proxy.go
func (lb *LoadBalancer) connectToShard(shardID int) {
    // OLD: net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
    // NEW:
    sockPath := fmt.Sprintf("/tmp/shard-%d.sock", shardID)
    conn, err := net.Dial("unix", sockPath)
    // ...
}

// ws/internal/shard/shard.go
func (s *Shard) Listen() {
    // OLD: listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
    // NEW:
    sockPath := fmt.Sprintf("/tmp/shard-%d.sock", s.id)
    listener, err := net.Listen("unix", sockPath)
    // ...
}
```

**Files to modify:**
- `ws/internal/loadbalancer/proxy.go`
- `ws/internal/shard/shard.go`

**Impact**: MEDIUM (20-30% proxy overhead reduction)
**Effort**: MEDIUM (2-3 days)
**Estimated gain**: +300-500 connections

---

### Opt #8: Goroutine Pooling 🔥

**Problem**: Creating/destroying goroutines per connection

**Solution**:
- Implement goroutine pool (e.g., via `ants` library or custom)
- Reuse goroutines for connection handling
- Reduces GC pressure and scheduler overhead

**Implementation** (using `github.com/panjf2000/ants`):

```go
// ws/internal/shard/handler.go
import "github.com/panjf2000/ants/v2"

type Shard struct {
    workerPool *ants.Pool
    // ...
}

func NewShard(...) *Shard {
    pool, _ := ants.NewPool(10000) // Pool size = max connections
    return &Shard{
        workerPool: pool,
        // ...
    }
}

func (s *Shard) HandleConnection(conn *websocket.Conn) {
    // Submit to pool instead of `go func()`
    s.workerPool.Submit(func() {
        s.handleConnectionWorker(conn)
    })
}
```

**Dependencies to add:**
```bash
go get -u github.com/panjf2000/ants/v2
```

**Files to modify:**
- `ws/internal/shard/handler.go` - Use goroutine pool for connection handling
- `ws/go.mod` - Add ants dependency

**Impact**: MEDIUM (15-25% goroutine overhead reduction)
**Effort**: MEDIUM (3-4 days)
**Estimated gain**: +400-600 connections

---

**Phase 2 Summary:**

| Optimization | Effort | Gain | Priority |
|-------------|--------|------|----------|
| **Shared Kafka pool** | **1 week** | **+800-1,200** | **P2 (HIGHEST)** |
| Smart message routing | 3-4 days | +400-600 | P2 |
| UDS for LB-shard | 2-3 days | +300-500 | P3 |
| Goroutine pooling | 3-4 days | +400-600 | P3 |
| **TOTAL** | **2-3 weeks** | **+1,900-2,900** | - |

**Expected Result**: 7,100-8,200 connections (49-73% improvement, **surpasses 3-shard baseline by 37-58%**)

---

## 💥 Phase 3: Architectural Changes (Week 7+ if needed)

**Only proceed if Phase 1+2 don't reach target (5,180+ connections)**

### Opt #9: Hybrid Shard Roles

**Problem**: All shards do everything (connections + Kafka)

**Solution**:
- **Connection shards** (5-6): Handle WebSocket connections only
- **Kafka shards** (1-2): Handle Kafka consumption and distribution
- Eliminates Kafka overhead from connection-handling shards

**Architecture**:

```
[Kafka Shard 0] ─┐
[Kafka Shard 1] ─┼→ [Shared Channel] → [Conn Shard 0-5]
                 │
                 └→ Process all Kafka messages
```

**Impact**: VERY HIGH (80%+ of current Kafka overhead eliminated)
**Effort**: HIGH (2-3 weeks)
**Estimated gain**: +1,500-2,000 connections

---

### Opt #10: Shared Memory Ring Buffer

**Problem**: Broadcasting copies message data to each shard

**Solution**:
- Use shared memory ring buffer for broadcasts
- Shards read directly from buffer (zero-copy)
- Eliminates per-shard message copying

**Impact**: HIGH (60-80% broadcast overhead reduction)
**Effort**: VERY HIGH (3-4 weeks)
**Estimated gain**: +800-1,200 connections

---

### Opt #11: Direct Shard Connection with Smart Routing

**Problem**: LoadBalancer proxying adds latency and overhead

**Solution**:
- LoadBalancer performs initial routing decision
- Client connects directly to assigned shard
- LoadBalancer tracks shard load for future routing

**Impact**: VERY HIGH (eliminates all proxy overhead)
**Effort**: HIGH (2 weeks)
**Tradeoffs**: Loses dynamic rebalancing, clients see multiple IPs
**Estimated gain**: +1,000-1,500 connections

---

**Phase 3 Summary:**

| Optimization | Effort | Gain | Priority |
|-------------|--------|------|----------|
| Hybrid shard roles | 2-3 weeks | +1,500-2,000 | P4 |
| Shared memory ring | 3-4 weeks | +800-1,200 | P5 |
| Direct shard connection | 2 weeks | +1,000-1,500 | P6 |
| **TOTAL** | **6-10 weeks** | **+3,300-4,700** | - |

**Expected Result**: 8,600-10,200 connections (81-115% improvement over 7-shard baseline)

---

## 📈 Success Criteria by Phase

| Phase | Target Connections | vs 3-Shard Baseline | vs 7-Shard Baseline | Status |
|-------|-------------------|---------------------|---------------------|--------|
| **Baseline (7 shards)** | 4,754 | -8.2% | 0% | ❌ |
| **Phase 1 Complete** | 5,500-5,800 | +6-12% | +16-22% | ✅ Surpasses 3-shard |
| **Phase 2 Complete** | 7,100-8,200 | +37-58% | +49-73% | 🚀 Major improvement |
| **Phase 3 (if needed)** | 8,600-10,200 | +66-97% | +81-115% | 🔥 Transformative |

---

## 🔬 Phase 0: Profile Before Optimizing

**Before starting optimizations, add profiling infrastructure:**

### Add pprof Endpoint

```go
// ws/internal/shared/handlers_http.go
import _ "net/http/pprof"

func setupHTTPHandlers(mux *http.ServeMux, /* ... */) {
    // Existing handlers...

    // Add pprof endpoints (development/staging only)
    if os.Getenv("ENABLE_PPROF") == "true" {
        mux.HandleFunc("/debug/pprof/", pprof.Index)
        mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
        mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
        mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
        mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
    }
}
```

### Run Profiling During Load Test

```bash
# Terminal 1: Start load test
task gcp:load-test:capacity

# Terminal 2: Collect CPU profile (30 seconds)
curl "http://34.70.240.105:3004/debug/pprof/profile?seconds=30" > cpu_7shards.prof

# Terminal 3: Collect heap profile
curl "http://34.70.240.105:3004/debug/pprof/heap" > heap_7shards.prof

# Analyze locally
go tool pprof -http=:8080 cpu_7shards.prof
```

### Key Metrics to Measure

- CPU time per component (Kafka, BroadcastBus, LoadBalancer proxy, connections)
- Goroutine count by type
- Memory allocations per operation
- Syscall overhead (cgroup reads, network I/O)

---

## 📊 Metrics to Track

Add these Prometheus metrics to measure optimization impact:

```go
// ws/internal/shared/monitoring/metrics.go
var (
    // CPU admission metrics
    cpuChecksTotal = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "cpu_checks_total",
            Help: "Total number of CPU checks performed",
        },
    )
    cpuChecksCached = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "cpu_checks_cached",
            Help: "Number of CPU checks served from cache",
        },
    )

    // Kafka metrics
    kafkaMessagesProcessed = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kafka_messages_processed_total",
            Help: "Total Kafka messages processed by shard",
        },
        []string{"shard"},
    )
    kafkaBatchSizeHistogram = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name: "kafka_batch_size",
            Help: "Kafka message batch size distribution",
            Buckets: prometheus.LinearBuckets(1, 10, 10), // 1, 11, 21, ..., 91
        },
    )

    // Broadcast metrics
    broadcastBatchSizeHistogram = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name: "broadcast_batch_size",
            Help: "Broadcast batch size distribution",
            Buckets: prometheus.LinearBuckets(1, 5, 10), // 1, 6, 11, ..., 46
        },
    )
    broadcastRoutedMessages = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "broadcast_routed_messages_total",
            Help: "Messages routed by type",
        },
        []string{"type"}, // "broadcast", "targeted"
    )

    // Goroutine pool metrics
    goroutinePoolUtilization = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "goroutine_pool_utilization",
            Help: "Goroutine pool utilization percentage",
        },
    )
    goroutinePoolQueueDepth = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "goroutine_pool_queue_depth",
            Help: "Goroutine pool queue depth",
        },
    )
)
```

---

## 🚀 Complete Impact Matrix

| Optimization | Impact | Effort | Priority | Estimated Gain |
|-------------|--------|--------|----------|----------------|
| Centralized CPU check | LOW | LOW | ⚡ P1 | +150-200 |
| Sampled CPU monitoring | MED | LOW | ⚡ P1 | +100-150 |
| Kafka batching | MED | LOW | ⚡ P1 | +300-400 |
| Broadcast batching | MED | LOW | ⚡ P1 | +200-300 |
| **Shared Kafka pool** | **HIGH** | **MED-HIGH** | 🔥 **P2** | **+800-1,200** |
| Smart message routing | MED-HIGH | MED | 🔥 P2 | +400-600 |
| UDS for LB-shard | MED | MED | 🔥 P3 | +300-500 |
| Goroutine pooling | MED | MED | 🔥 P3 | +400-600 |
| Hybrid shard roles | VERY HIGH | HIGH | 💥 P4 | +1,500-2,000 |
| Shared memory ring | HIGH | VERY HIGH | 💥 P5 | +800-1,200 |
| Direct shard connection | VERY HIGH | HIGH | 💥 P6 | +1,000-1,500 |

---

## 🎯 Recommended Implementation Path

### Week 0: Profile & Baseline
1. Add pprof endpoints
2. Profile 7-shard config under load
3. Identify top CPU consumers
4. Create optimization priority based on data

**Target**: Quantify exactly where CPU is spent

---

### Week 1-2: Phase 1 - Quick Wins
1. Centralized CPU admission control (2-3h)
2. Sampled CPU monitoring (1h)
3. Kafka message batching (2h)
4. Broadcast message batching (3h)

**Expected gain**: 15-20% overhead reduction → ~5,600 connections (vs 4,754)

---

### Week 3-6: Phase 2 - High-ROI Changes
1. **Shared Kafka consumer pool** (1 week) - PRIORITY
2. Smart message routing (3-4 days)
3. Goroutine pooling (3-4 days)
4. UDS for LB-shard (2-3 days) - Optional

**Expected gain**: Additional 30-40% → ~7,200-8,000 connections

---

### Week 7+: Phase 3 - Architecture Changes (if needed)
Based on Phase 1-2 results, decide if needed:
- If 7,200+ connections achieved → SUCCESS, stop here
- If still below 3-shard performance → Implement hybrid shard roles

**Target**: Match or exceed 3-shard performance (5,180 connections)

---

## 📝 Notes

### Existing Infrastructure

The codebase already has:
- ✅ Prometheus metrics collection (`/metrics` endpoint)
- ✅ Health check endpoint (`/health` with rich stats)
- ✅ System resource monitoring (CPU, memory, goroutines)
- ✅ Load testing infrastructure (`loadtest/main.go`)
- ❌ No pprof endpoint (needs to be added)
- ❌ No benchmark tests (needs to be added)

### Risk Assessment

**Low Risk (Phase 1):**
- All optimizations are performance improvements without architectural changes
- Easy to rollback if issues occur
- No external API changes

**Medium Risk (Phase 2):**
- Shared Kafka pool requires careful testing (highest impact but also highest risk)
- Message routing changes require validation of message delivery guarantees
- UDS limits deployment flexibility (same-host only)

**High Risk (Phase 3):**
- Architectural changes require significant testing
- Direct shard connection changes client behavior
- Shared memory requires platform-specific code

---

## 🔗 Related Documentation

- [Session Summary: IPv4 Shard Investigation](./sessions/session-summary-2025-11-12-ipv4-shard-investigation.md)
- [Session Summary: 7-Shard Performance Analysis](./sessions/session-summary-2025-11-12-2310.md)
- Health Endpoint: `ws/internal/shared/handlers_http.go`
- Metrics: `ws/internal/shared/monitoring/metrics.go`
- Load Testing: `loadtest/main.go`

---

**Last Updated**: 2025-11-13
**Status**: Planning Phase
**Next Action**: Add pprof endpoint and profile current 7-shard configuration
