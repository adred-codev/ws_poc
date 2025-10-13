# WebSocket Server Capacity Planning

> **Comprehensive reference for calculating resource requirements and scaling parameters**

## Parameter Interdependencies

When scaling the WebSocket server, **six parameters must be updated together**:

```
WS_MAX_CONNECTIONS (input)
    ↓
WS_WORKER_POOL_SIZE (scales with connections)
    ↓
WS_WORKER_QUEUE_SIZE (100× workers)
    ↓
WS_MAX_GOROUTINES (connections×2 + workers + overhead)

WS_MAX_NATS_RATE (fixed at 25)
WS_MAX_BROADCAST_RATE (fixed at 25)
```

## Scaling Formulas

### 1. WS_MAX_CONNECTIONS
**What it is:** Maximum concurrent WebSocket connections
**How to set:** Based on your capacity requirements

**Examples:** 500, 1000, 1500, 2000

---

### 2. WS_WORKER_POOL_SIZE
**What it is:** Number of goroutines handling broadcast message fanout
**Why it scales:** Each broadcast must write to ALL connections. Workers pull from queue and write to connections.

**Formula (Production-Validated):**
```
workers = max(32, connections / 40)
```

**Reasoning:**
- Workers are I/O-bound (blocked on network writes)
- Target: ~300-1,000 work items per worker per second
- Work items per second: connections × broadcast_rate = connections × 25
- Workers needed: (connections × 25) / 1000 ≈ connections / 40
- Minimum 32 workers (proven stable at 500 connections)
- **Formula evolution**: Initial testing (500-2K) used `connections/16`, but production-scale testing (7K-10K) validated `connections/40` as optimal

**Examples:**

| Connections | Calculation | Workers | Work Items/Worker/Sec |
|-------------|-------------|---------|----------------------|
| 500         | max(32, 500/40) | 32 | 391 |
| 1,000       | max(32, 1000/40) | 32 | 781 |
| 2,000       | max(32, 2000/40) | 64 | 781 |
| 7,000       | max(32, 7000/40) | 192 | 911 |
| 10,000      | max(32, 10000/40) | 256 | 977 |

**Impact on resources:**
- CPU: Minimal (workers are I/O-blocked, not CPU-bound)
- Memory: workers × 8KB stack = 128 workers × 8KB = 1MB
- Goroutines: Workers are part of static goroutine count

---

### 3. WS_WORKER_QUEUE_SIZE
**What it is:** Buffer size for broadcast work queue
**Why it scales:** Queue must hold pending writes during bursts

**Formula:**
```
queue_size = workers × 100
```

**Reasoning:**
- 100× multiplier provides buffer for burst traffic
- At 25 broadcasts/sec, each broadcast queues work for ALL connections
- Queue throughput: connections × broadcast_rate items/sec
- Example: 7,000 conns × 25 msg/sec = 175,000 items/sec
- With 192 workers processing 175,000 items/sec, queue stays near-empty
- 100× buffer (19,200 slots) handles temporary bursts

**Examples:**

| Connections | Workers | Queue Size | Burst Capacity |
|-------------|---------|------------|---------------|
| 500         | 32      | 3,200      | 0.26 sec @ max rate |
| 1,000       | 32      | 3,200      | 0.13 sec @ max rate |
| 2,000       | 64      | 6,400      | 0.26 sec @ max rate |
| 7,000       | 192     | 19,200     | 0.11 sec @ max rate |
| 10,000      | 256     | 25,600     | 0.10 sec @ max rate |

---

### 4. WS_MAX_NATS_RATE
**What it is:** Maximum NATS messages consumed per second (rate limit)
**Why it's fixed:** This is a safety limit to prevent overload, NOT a scaling parameter

**Formula (Production-Validated):**
```
WS_MAX_NATS_RATE = 25  (fixed)
```

**Reasoning:**
- Each NATS message = 1 broadcast to ALL connections
- Based on production metrics: 280K users, 40K tx/day peak
- Actual publish rates: 5 msg/sec (average) to 19.7 msg/sec (high peak)
- Setting: 25 msg/sec provides 25% headroom above peak
- **Does NOT scale with connections** (more connections = same NATS rate, more total writes)
- **Previous setting**: 20 msg/sec (based on early estimates)

**Total outbound messages:**
```
total_outbound = WS_MAX_NATS_RATE × connections
               = 25 × connections
```

---

### 5. WS_MAX_BROADCAST_RATE
**What it is:** Maximum broadcasts processed per second (rate limit)
**Why it matches NATS rate:** 1 NATS message = 1 broadcast

**Formula:**
```
WS_MAX_BROADCAST_RATE = 25  (fixed, must equal WS_MAX_NATS_RATE)
```

---

### 6. WS_MAX_GOROUTINES
**What it is:** Hard limit on total goroutines (prevents runaway goroutine leaks)
**Why it scales:** More connections = more goroutines (readPump + writePump per connection)

**Formula:**
```
goroutines = ((connections × 2) + static + overhead) × 1.2
static = workers + 13
      = workers + monitors(5) + runtime(8)

Combined:
goroutines = ((connections × 2) + workers + 13) × 1.2
```

**Breakdown:**
- **Per connection:** 2 goroutines (readPump + writePump)
- **Static goroutines:**
  - Workers: Variable (see WS_WORKER_POOL_SIZE)
  - Monitors: 5 (resource monitors, health checks)
  - Runtime: 8 (Go runtime, garbage collector)
- **Overhead:** 20% buffer for transient goroutines

**Examples:**

| Connections | Workers | Calculation | Goroutines |
|-------------|---------|-------------|------------|
| 500         | 32      | ((500×2) + 32 + 13) × 1.2 | 1,254 → **1,300** |
| 1,000       | 64      | ((1000×2) + 64 + 13) × 1.2 | 2,492 → **2,500** |
| 1,500       | 96      | ((1500×2) + 96 + 13) × 1.2 | 3,731 → **3,800** |
| 2,000       | 128     | ((2000×2) + 128 + 13) × 1.2 | 4,969 → **5,000** |

**Memory cost:** goroutines × 8KB stack
- 5,000 goroutines × 8KB = 40MB (5.2% of 768MB limit)

---

## Message Flow Architecture

### How Messages Flow
```
Publisher → NATS (25 msg/sec) → WS Server → Broadcast to ALL clients
```

**Key Insight:** 1 NATS message = 1 broadcast to ALL connected clients

### Message Rate Calculations

**Per-client message rate:** Each client receives every NATS message
```
messages_per_client = WS_MAX_NATS_RATE = 25 msg/sec (constant)
```

**Total outbound messages:** Scales linearly with connections
```
total_outbound = WS_MAX_NATS_RATE × connections
               = 25 × connections
```

**Examples:**

| Connections | NATS Rate | Per-Client Rate | Total Outbound |
|-------------|-----------|-----------------|----------------|
| 500         | 25/sec    | 25/sec          | 12,500/sec     |
| 1,000       | 25/sec    | 25/sec          | 25,000/sec     |
| 2,000       | 25/sec    | 25/sec          | 50,000/sec     |
| 7,000       | 25/sec    | 25/sec          | 175,000/sec    |
| 10,000      | 25/sec    | 25/sec          | 250,000/sec    |

---

## Resource Usage Scaling

### CPU Usage

**What drives CPU:**
- JSON marshaling (encoding messages)
- Network I/O (socket writes)
- Goroutine context switching

**Scaling factor:** Linear with total outbound messages

**Formula:**
```
CPU_new = CPU_baseline × (connections_new / connections_baseline)
```

**Measured baseline (500 connections):**
- Total outbound: 10,000 msg/sec
- CPU usage: 11%

**Projected:**

| Connections | Total Outbound | CPU (estimated) | CPU Headroom |
|-------------|----------------|-----------------|--------------|
| 500         | 10,000/sec     | 11%            | 64% (75% - 11%) |
| 1,000       | 20,000/sec     | 22%            | 53% |
| 1,500       | 30,000/sec     | 33%            | 42% |
| 2,000       | 40,000/sec     | 44%            | 31% |

**CPU Threshold:** 75% (WS_CPU_REJECT_THRESHOLD) - server rejects new connections above this

---

### Memory Usage

**Components:**
1. **Goroutine stacks:** goroutines × 8KB
2. **Connection buffers:** connections × BufferSize (if configured)
3. **Worker queue:** queue_size × avg_message_size
4. **Runtime overhead:** ~20-30MB

**Formula:**
```
memory_mb ≈ (goroutines × 0.008) + overhead
```

**Examples:**

| Connections | Goroutines | Memory (estimated) | Memory % (of 768MB) |
|-------------|------------|-------------------|---------------------|
| 500         | 1,300      | 38 MB             | 5%                  |
| 1,000       | 2,500      | 77 MB             | 10%                 |
| 1,500       | 3,800      | 115 MB            | 15%                 |
| 2,000       | 5,000      | 154 MB            | 20%                 |

**Memory Threshold:** 768MB (WS_MEMORY_LIMIT) - server unhealthy if exceeded

---

## Scaling Publish Rate (NATS Rate)

**Common Question**: If we increase the publish rate (e.g., from 25 → 50 msg/sec), do we need to increase other config values and vertically scale the server?

**Answer**: It depends on how much you increase it. The key constraint is **CPU capacity**, not memory.

### What Changes with Publish Rate

#### ✅ Must Always Scale (Config Changes Only)

**1. Worker Pool Size**
```
Formula: workers = (connections × publish_rate) / 1000

At 25 msg/sec: (7,000 × 25) / 1000 = 175 → 192 workers
At 50 msg/sec: (7,000 × 50) / 1000 = 350 → 384 workers
At 100 msg/sec: (7,000 × 100) / 1000 = 700 → 768 workers

Reason: Workers become CPU-bound if load exceeds ~1,000 items/sec per worker
```

**2. Worker Queue Size**
```
Formula: queue = workers × 100

At 50 msg/sec: 384 × 100 = 38,400
At 100 msg/sec: 768 × 100 = 76,800

Reason: Scales automatically with workers, maintains same burst capacity
```

**3. Goroutine Limit**
```
Formula: ((connections × 2) + workers + 13) × 1.2

At 50 msg/sec: ((7,000 × 2) + 384 + 13) × 1.2 = 17,276
At 100 msg/sec: ((7,000 × 2) + 768 + 13) × 1.2 = 17,737

Reason: Increases with worker count (minimal impact)
```

#### ⚠️ May Need Vertical Scaling (Hardware Upgrade)

**1. CPU Capacity** - The Real Bottleneck
```
CPU usage scales with: total_writes = connections × publish_rate

Current (e2-standard-2, 2 vCPU):
7,000 conns × 25 msg/sec = 175,000 writes/sec = 47% CPU
Measured rate: 47% / 175K = 0.0002686% per 1K writes

At 50 msg/sec:
7,000 × 50 = 350,000 writes/sec
CPU: 350K × 0.0002686% = 94% ⚠️ Near limit, minimal headroom

At 80 msg/sec:
7,000 × 80 = 560,000 writes/sec
CPU: 560K × 0.0002686% = 150% ❌ Exceeds 2 vCPU capacity!

Safe maximum on e2-standard-2: ~80 msg/sec
```

**2. Network Bandwidth** - Only at Extreme Rates
```
Current: 175K writes/sec × 500 bytes = 87.5 MB/sec = 700 Mbps ✅

At 100 msg/sec:
700K writes/sec × 500 bytes = 350 MB/sec = 2.8 Gbps ⚠️
Exceeds e2-standard-2 NIC (2 Gbps)

Safe maximum on 2 Gbps NIC: ~100 msg/sec
```

#### ✅ Does NOT Scale with Publish Rate

- **Memory**: Still ~0.7MB per connection (independent of message rate)
- **Connection buffers**: Same size regardless of rate
- **Goroutines**: Only increases slightly with workers

### Three Strategies for Higher Publish Rates

#### Strategy 1: **Config-Only Scaling** (No Hardware Change)
**When**: Moderate increase (25 → 50 msg/sec)

```yaml
# Changes needed:
WS_MAX_NATS_RATE: 25 → 50
WS_WORKER_POOL_SIZE: 192 → 384
WS_WORKER_QUEUE_SIZE: 19,200 → 38,400
WS_MAX_GOROUTINES: 17,500 → 18,000

# Resources at 7K connections, 50 msg/sec:
CPU: ~94% (near limit but functional)
Memory: 83% (safe)
Instance: e2-standard-2 (no change)
Cost: $24/month (no change)
```

**Trade-off**: CPU runs hot (90-95%), minimal headroom for spikes

---

#### Strategy 2: **Reduce Connections** (No Hardware Change)
**When**: Much higher rate but fewer concurrent users needed

```yaml
# Keep CPU budget constant:
Target: 175,000 writes/sec (same as current)

# At 100 msg/sec, reduce connections:
connections = 175,000 / 100 = 1,750

# Configuration:
WS_MAX_CONNECTIONS: 7,000 → 1,750
WS_MAX_NATS_RATE: 25 → 100
WS_WORKER_POOL_SIZE: 192 → 64
WS_WORKER_QUEUE_SIZE: 19,200 → 6,400
WS_MAX_GOROUTINES: 17,500 → 4,500

# Resources:
CPU: 47% (same as before)
Memory: 35% (1,225 MB connections + 974 MB overhead)
Instance: e2-standard-2 (no change)
Cost: $24/month (no change)
```

**Trade-off**: Support fewer users but with higher message rate per user

---

#### Strategy 3: **Vertical Scaling** (Hardware Upgrade)
**When**: Need BOTH high rate AND high connections

```yaml
# Upgrade: e2-standard-2 → e2-standard-4
# Resources: 2 vCPU → 4 vCPU, 8GB → 16GB, 2 Gbps → 4 Gbps

# At 7K connections, 100 msg/sec:
WS_MAX_CONNECTIONS: 7,000 (unchanged)
WS_MAX_NATS_RATE: 25 → 100
WS_WORKER_POOL_SIZE: 192 → 768
WS_WORKER_QUEUE_SIZE: 19,200 → 76,800
WS_MAX_GOROUTINES: 17,500 → 35,000
WS_MEMORY_LIMIT: 7GB → 14GB

# Resources at 7K connections, 100 msg/sec:
CPU: ~94% of 4 vCPU = 47% per core (same as current)
Memory: 42% (5.8GB / 14GB)
Network: 2.8 Gbps (fits in 4 Gbps NIC)
Instance: e2-standard-4
Cost: $72/month (+$48)
```

**Trade-off**: Higher monthly cost, but maintains same CPU per-core utilization

---

### Decision Matrix: When to Scale Vertically

| Publish Rate | Connections | Strategy | Hardware | Monthly Cost | Notes |
|--------------|-------------|----------|----------|--------------|-------|
| **25 msg/sec** | 7,000 | Current | e2-standard-2 | $24 | ✅ Production baseline |
| **50 msg/sec** | 7,000 | Config only | e2-standard-2 | $24 | ⚠️ CPU 94%, runs hot |
| **80 msg/sec** | 7,000 | Config only | e2-standard-2 | $24 | ⚠️ CPU 150%, at limit |
| **100 msg/sec** | 7,000 | Vertical scale | e2-standard-4 | $72 | ✅ Recommended |
| **100 msg/sec** | 1,750 | Reduce conns | e2-standard-2 | $24 | ✅ Alternative |
| **200 msg/sec** | 7,000 | Vertical scale | e2-standard-8 | $144 | For extreme rates |

### The Key Formula

**CPU Capacity Check** (most important):
```
total_writes_per_sec = connections × publish_rate
cpu_usage = total_writes_per_sec × 0.0002686%

On e2-standard-2 (2 vCPU, 150% usable):
safe_max_writes = 150% / 0.0002686% = ~560,000 writes/sec

At 7,000 connections:
safe_max_rate = 560,000 / 7,000 = ~80 msg/sec

Beyond 80 msg/sec → Vertical scaling required
```

### Summary

**You're right that higher publish rate requires config changes**, but:
- **25 → 50 msg/sec**: Just config changes (increase workers), no hardware upgrade
- **25 → 80 msg/sec**: Config changes, CPU runs at limit (~150%)
- **25 → 100+ msg/sec**: YES, vertical scaling required OR reduce connections

**The bottleneck is CPU**, not memory. Memory usage stays constant at ~0.7MB per connection regardless of publish rate.

---

## Complete Configuration Matrix

**Small-scale configurations for e2-small instance (1.5 CPU, 768MB RAM):**

| Connections | Workers | Queue | Goroutines | NATS Rate | Broadcast Rate | CPU  | Memory | Status |
|-------------|---------|-------|------------|-----------|----------------|------|--------|--------|
| **500**     | 32      | 3,200 | 1,300      | 25        | 25             | 14%  | 5%     | ✅ Safe |
| **1,000**   | 32      | 3,200 | 2,500      | 25        | 25             | 28%  | 10%    | ✅ Safe |
| **2,000**   | 64      | 6,400 | 4,900      | 25        | 25             | 56%  | 20%    | ✅ Safe |

**Production-scale configurations for e2-standard-2 (2 vCPU, 8GB RAM):**

| Connections | Workers | Queue  | Goroutines | NATS Rate | Broadcast Rate | CPU | Memory | Status |
|-------------|---------|--------|------------|-----------|----------------|-----|--------|--------|
| **7,000**   | 192     | 19,200 | 17,500     | 25        | 25             | 47% | 81%    | ✅ **Production** |
| **8,849**   | 256     | 25,600 | 21,700     | 25        | 25             | 60% | 99%    | ⚠️ At limit |

**Legend:**
- ✅ Safe: Well below all thresholds
- ⚠️ At limit: Operating at memory capacity
- **Production**: Recommended production configuration

**Note**: For configurations >2K connections, use e2-standard-2 or larger. See IMPLEMENTATION_PLAN.md for detailed capacity analysis.

---

## Scaling Procedure

### Step-by-Step: Scaling from 500 to 7,000 Connections

**1. Calculate new parameters:**

```bash
connections=7000

# Workers: max(32, connections/40)
workers=$(( connections / 40 ))
[[ $workers -lt 32 ]] && workers=32

# Queue: workers × 100
queue=$(( workers * 100 ))

# Goroutines: ((connections×2) + workers + 13) × 1.2, rounded up
goroutines=$(( ((connections * 2) + workers + 13) * 12 / 10 ))
# Round up to nearest 100
goroutines=$(( (goroutines + 99) / 100 * 100 ))

echo "WS_MAX_CONNECTIONS=$connections"
echo "WS_WORKER_POOL_SIZE=$workers"
echo "WS_WORKER_QUEUE_SIZE=$queue"
echo "WS_MAX_GOROUTINES=$goroutines"
echo "WS_MAX_NATS_RATE=25"
echo "WS_MAX_BROADCAST_RATE=25"
```

**Output:**
```
WS_MAX_CONNECTIONS=7000
WS_WORKER_POOL_SIZE=175
WS_WORKER_QUEUE_SIZE=17500
WS_MAX_GOROUTINES=17000
WS_MAX_NATS_RATE=25
WS_MAX_BROADCAST_RATE=25
```

**2. Update docker-compose.yml:**

```yaml
environment:
  # Static resource configuration (explicit limits)
  - WS_CPU_LIMIT=1.9
  - WS_MEMORY_LIMIT=7516192768  # 7GB in bytes
  - WS_MAX_CONNECTIONS=7000     # ← UPDATED (production target)

  # Worker pool sizing:
  #   Formula: max(32, connections/40)
  #   Calculation: max(32, 7000/40) = 175 workers (rounded to 192, power of 2)
  #   Load per worker: (7000 × 25) / 192 = 911 items/sec
  - WS_WORKER_POOL_SIZE=192     # ← UPDATED
  - WS_WORKER_QUEUE_SIZE=19200  # ← UPDATED (192 × 100)

  # Rate limiting (prevent overload) - FIXED VALUES
  # Based on production metrics: 5-20 msg/sec typical, 25 provides headroom
  - WS_MAX_NATS_RATE=25         # Fixed rate limit
  - WS_MAX_BROADCAST_RATE=25    # Fixed rate limit

  # Goroutine limit calculation:
  #   Formula: ((MAX_CONNECTIONS × 2) + STATIC + OVERHEAD) × 1.2
  #   Static: workers(192) + monitors(5) + runtime(8) = 205
  #   Calculation: ((7000 × 2) + 205) × 1.2 = 17,046 → 17500
  #   Memory cost: 17,500 × 8KB = 140MB (1.9% of 7GB RAM)
  #   CPU cost: Only active goroutines use CPU (most idle on I/O)
  - WS_MAX_GOROUTINES=17500     # ← UPDATED

  # Safety thresholds - FIXED VALUES
  - WS_CPU_REJECT_THRESHOLD=75.0
  - WS_CPU_PAUSE_THRESHOLD=80.0
```

**3. Build, deploy, test:**

```bash
# Build and deploy
task gcp:deploy:update

# Run capacity test (at design limit)
TARGET_CONNECTIONS=7000 RAMP_RATE=100 DURATION=1800 task gcp:test:sustained

# Run stress test (intentional overload)
TARGET_CONNECTIONS=10000 RAMP_RATE=100 DURATION=600 task gcp:test:sustained
```

**4. Verify health:**

```bash
# Check health endpoint
task gcp:health

# Expected output:
# {
#   "healthy": true,
#   "status": "degraded",
#   "warnings": ["Server at full capacity (7000/7000)"],
#   "checks": {
#     "cpu": {"percentage": 47, "healthy": true},
#     "memory": {"percentage": 81, "healthy": true},
#     "goroutines": {"current": 14205, "limit": 17500, "healthy": true}
#   }
# }
```

---

## Health Check Thresholds

**Server reports UNHEALTHY when:**
1. **NATS connection lost** (critical dependency)
2. **CPU > 75%** (WS_CPU_REJECT_THRESHOLD)
3. **Memory > 768MB** (WS_MEMORY_LIMIT)
4. **Goroutines > configured limit** (WS_MAX_GOROUTINES)

**Server reports DEGRADED (warnings) when:**
1. **Capacity at 100%** (operating at design limit, still healthy)
2. **Capacity > 90%** (consider scaling)
3. **CPU > 70%** (monitor closely)
4. **Memory > 80%** (monitor closely)
5. **Goroutines > 80%** (monitor closely)

---

## Quick Reference: Scaling Formulas

**Copy-paste for spreadsheet calculations:**

```
# Input
connections = 7000

# Derived parameters (production-validated formulas)
workers = MAX(32, connections / 40)
queue_size = workers × 100
static_goroutines = workers + 13
goroutines = ((connections × 2) + static_goroutines) × 1.2

# Fixed parameters (do not scale)
nats_rate = 25
broadcast_rate = 25
cpu_threshold = 75
cpu_pause = 80

# Resource estimates
total_outbound = connections × nats_rate
cpu_percent = 14% × (connections / 500)  # Baseline: 14% @ 500 conns with 25 msg/sec
memory_per_conn = 0.7  # MB per connection (measured at scale)
memory_mb = (connections × memory_per_conn) + 974  # 974MB overhead
memory_percent = memory_mb / 7168 × 100  # For e2-standard-2 (7GB)
```

---

## Testing Recommendations

### Capacity Test (At Design Limit)
**Purpose:** Verify server handles maximum configured load

```bash
TARGET_CONNECTIONS=<WS_MAX_CONNECTIONS> RAMP_RATE=100 DURATION=1800 task gcp:test:sustained
```

**Expected:**
- All connections accepted (100% success rate)
- Server stable for full duration (30 min)
- CPU < 75%
- Memory < 768MB
- Goroutines < limit
- Status: `degraded` with warning "Server at full capacity"

### Stress Test (Intentional Overload)
**Purpose:** Verify server correctly rejects excess load without crashing

```bash
TARGET_CONNECTIONS=$((WS_MAX_CONNECTIONS * 1.5)) RAMP_RATE=150 DURATION=600 task gcp:test:sustained
```

**Expected:**
- 50% connections rejected (server at capacity)
- Server remains stable (no crash)
- Accepted connections remain healthy
- CPU < 75%
- Memory < 768MB

---

**Last updated:** 2025-10-13
**Production-scale testing:** 8,849 connections @ 60% CPU, 99% memory (88.5% success rate)
**Production target:** 7,000 connections @ 47% CPU, 81% memory (99%+ success rate)
**Instance type:** GCP e2-standard-2 (2 vCPU, 8GB RAM)
**Production metrics:** 280K users, 40K tx/day peak → 5-20 msg/sec publish rate
