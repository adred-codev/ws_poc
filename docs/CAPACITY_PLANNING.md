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

WS_MAX_NATS_RATE (fixed at 20)
WS_MAX_BROADCAST_RATE (fixed at 20)
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

**Formula:**
```
workers = max(32, connections / 16)
```

**Reasoning:**
- Workers are I/O-bound (blocked on network writes)
- Target: ~300-500 work items per worker per second
- Work items per second: connections × broadcast_rate = connections × 20
- Workers needed: (connections × 20) / 400 ≈ connections / 20
- Rounded to connections / 16 for safety margin
- Minimum 32 workers (proven stable at 500 connections)

**Examples:**

| Connections | Calculation | Workers | Work Items/Worker/Sec |
|-------------|-------------|---------|----------------------|
| 500         | max(32, 500/16) | 32 | 313 |
| 1,000       | max(32, 1000/16) | 64 | 313 |
| 1,500       | max(32, 1500/16) | 96 | 313 |
| 2,000       | max(32, 2000/16) | 128 | 313 |

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
- At 20 broadcasts/sec, each broadcast queues work for ALL connections
- Queue throughput: connections × broadcast_rate items/sec
- Example: 2,000 conns × 20 msg/sec = 40,000 items/sec
- With 128 workers processing 40,000 items/sec, queue stays near-empty
- 100× buffer (12,800 slots) handles temporary bursts

**Examples:**

| Connections | Workers | Queue Size | Burst Capacity |
|-------------|---------|------------|---------------|
| 500         | 32      | 3,200      | 0.32 sec @ max rate |
| 1,000       | 64      | 6,400      | 0.32 sec @ max rate |
| 1,500       | 96      | 9,600      | 0.32 sec @ max rate |
| 2,000       | 128     | 12,800     | 0.32 sec @ max rate |

---

### 4. WS_MAX_NATS_RATE
**What it is:** Maximum NATS messages consumed per second (rate limit)
**Why it's fixed:** This is a safety limit to prevent overload, NOT a scaling parameter

**Formula:**
```
WS_MAX_NATS_RATE = 20  (fixed)
```

**Reasoning:**
- Each NATS message = 1 broadcast to ALL connections
- At 20 msg/sec with 2,000 connections = 40,000 client writes/sec
- This is the incoming message rate limit
- **Does NOT scale with connections** (more connections = same NATS rate, more total writes)

**Total outbound messages:**
```
total_outbound = WS_MAX_NATS_RATE × connections
               = 20 × connections
```

---

### 5. WS_MAX_BROADCAST_RATE
**What it is:** Maximum broadcasts processed per second (rate limit)
**Why it matches NATS rate:** 1 NATS message = 1 broadcast

**Formula:**
```
WS_MAX_BROADCAST_RATE = 20  (fixed, must equal WS_MAX_NATS_RATE)
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
Publisher → NATS (20 msg/sec) → WS Server → Broadcast to ALL clients
```

**Key Insight:** 1 NATS message = 1 broadcast to ALL connected clients

### Message Rate Calculations

**Per-client message rate:** Each client receives every NATS message
```
messages_per_client = WS_MAX_NATS_RATE = 20 msg/sec (constant)
```

**Total outbound messages:** Scales linearly with connections
```
total_outbound = WS_MAX_NATS_RATE × connections
               = 20 × connections
```

**Examples:**

| Connections | NATS Rate | Per-Client Rate | Total Outbound |
|-------------|-----------|-----------------|----------------|
| 500         | 20/sec    | 20/sec          | 10,000/sec     |
| 1,000       | 20/sec    | 20/sec          | 20,000/sec     |
| 1,500       | 20/sec    | 20/sec          | 30,000/sec     |
| 2,000       | 20/sec    | 20/sec          | 40,000/sec     |

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

## Complete Configuration Matrix

**Safe configurations for e2-small instance (1.5 CPU, 768MB RAM):**

| Connections | Workers | Queue | Goroutines | NATS Rate | Broadcast Rate | CPU  | Memory | Status |
|-------------|---------|-------|------------|-----------|----------------|------|--------|--------|
| **500**     | 32      | 3,200 | 1,300      | 20        | 20             | 11%  | 5%     | ✅ Current |
| **1,000**   | 64      | 6,400 | 2,500      | 20        | 20             | 22%  | 10%    | ✅ Recommended |
| **1,500**   | 96      | 9,600 | 3,800      | 20        | 20             | 33%  | 15%    | ✅ Safe |
| **2,000**   | 128     | 12,800| 5,000      | 20        | 20             | 44%  | 20%    | ✅ Safe |
| **2,500**   | 160     | 16,000| 6,300      | 20        | 20             | 55%  | 25%    | ⚠️ Moderate |
| **3,000**   | 192     | 19,200| 7,500      | 20        | 20             | 66%  | 30%    | ⚠️ Near limit |

**Legend:**
- ✅ Safe: Well below all thresholds
- ⚠️ Moderate: Approaching CPU threshold (75%)
- ❌ Unsafe: Exceeds resource limits

---

## Scaling Procedure

### Step-by-Step: Scaling from 500 to 2,000 Connections

**1. Calculate new parameters:**

```bash
connections=2000

# Workers: max(32, connections/16)
workers=$(( connections / 16 ))
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
echo "WS_MAX_NATS_RATE=20"
echo "WS_MAX_BROADCAST_RATE=20"
```

**Output:**
```
WS_MAX_CONNECTIONS=2000
WS_WORKER_POOL_SIZE=128
WS_WORKER_QUEUE_SIZE=12800
WS_MAX_GOROUTINES=5000
WS_MAX_NATS_RATE=20
WS_MAX_BROADCAST_RATE=20
```

**2. Update docker-compose.yml:**

```yaml
environment:
  # Static resource configuration (explicit limits)
  - WS_CPU_LIMIT=1.5
  - WS_MEMORY_LIMIT=805306368  # 768MB in bytes
  - WS_MAX_CONNECTIONS=2000    # ← UPDATED

  # Worker pool sizing:
  #   Formula: max(32, connections/16)
  #   Calculation: max(32, 2000/16) = 128 workers
  #   Load per worker: (2000 × 20) / 128 = 313 items/sec
  - WS_WORKER_POOL_SIZE=128     # ← UPDATED
  - WS_WORKER_QUEUE_SIZE=12800  # ← UPDATED (128 × 100)

  # Rate limiting (prevent overload) - FIXED VALUES
  - WS_MAX_NATS_RATE=20        # Fixed rate limit
  - WS_MAX_BROADCAST_RATE=20   # Fixed rate limit

  # Goroutine limit calculation:
  #   Formula: ((MAX_CONNECTIONS × 2) + STATIC + OVERHEAD) × 1.2
  #   Static: workers(128) + monitors(5) + runtime(8) = 141
  #   Calculation: ((2000 × 2) + 141) × 1.2 = 4,969 → 5000
  #   Memory cost: 5,000 × 8KB = 40MB (5.2% of 768MB RAM)
  #   CPU cost: Only active goroutines use CPU (most idle on I/O)
  - WS_MAX_GOROUTINES=5000     # ← UPDATED

  # Safety thresholds - FIXED VALUES
  - WS_CPU_REJECT_THRESHOLD=75.0
  - WS_CPU_PAUSE_THRESHOLD=80.0
```

**3. Build, deploy, test:**

```bash
# Build and deploy
task gcp:deploy:update

# Run capacity test (at design limit)
TARGET_CONNECTIONS=2000 RAMP_RATE=100 DURATION=1800 task gcp:test:sustained

# Run stress test (intentional overload)
TARGET_CONNECTIONS=3000 RAMP_RATE=150 DURATION=600 task gcp:test:sustained
```

**4. Verify health:**

```bash
# Check health endpoint
task gcp:health

# Expected output:
# {
#   "healthy": true,
#   "status": "degraded",
#   "warnings": ["Server at full capacity (2000/2000)"],
#   "checks": {
#     "cpu": {"percentage": 44, "healthy": true},
#     "memory": {"percentage": 20, "healthy": true},
#     "goroutines": {"current": 4141, "limit": 5000, "healthy": true}
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
connections = 2000

# Derived parameters
workers = MAX(32, connections / 16)
queue_size = workers × 100
static_goroutines = workers + 13
goroutines = ((connections × 2) + static_goroutines) × 1.2

# Fixed parameters (do not scale)
nats_rate = 20
broadcast_rate = 20
cpu_threshold = 75
cpu_pause = 80

# Resource estimates
total_outbound = connections × nats_rate
cpu_percent = 11% × (connections / 500)
memory_mb = goroutines × 0.008 + 20
memory_percent = memory_mb / 768 × 100
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

**Last updated:** 2025-10-08
**Tested baseline:** 500 connections @ 11% CPU, 5% memory
**Instance type:** GCP e2-small (1.5 vCPU, 768MB RAM)
