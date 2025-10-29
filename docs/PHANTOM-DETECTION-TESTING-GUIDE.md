# Phantom Connection Detection - Testing Guide

## Quick Start

### Test 100 Connections (5 min baseline)
```bash
task gcp2:test:validate:100
```

### Test 1,000 Connections (10 min scale test)
```bash
task gcp2:test:validate:1k
```

---

## What These Tests Do

Both tests perform **continuous phantom connection monitoring** by comparing three data sources:

```
┌─────────────────────────────────────────────────────────┐
│                 PHANTOM DETECTION FLOW                  │
└─────────────────────────────────────────────────────────┘
                            ↓
    ┌─────────────────────────────────────────────────┐
    │ 1. TCP GROUND TRUTH (Docker container)         │
    │    ssh → docker exec → netstat                 │
    │    → Actual ESTABLISHED connections            │
    └─────────────────────────────────────────────────┘
                            ↓
    ┌─────────────────────────────────────────────────┐
    │ 2. SERVER REPORTED (Health endpoint)           │
    │    curl /health → .checks.capacity.current     │
    │    → What server thinks it has                 │
    └─────────────────────────────────────────────────┘
                            ↓
    ┌─────────────────────────────────────────────────┐
    │ 3. CLIENT EXPERIENCE (Test runner metrics)     │
    │    • Messages received                         │
    │    • Heartbeats sent/acknowledged              │
    │    • Errors encountered                        │
    └─────────────────────────────────────────────────┘
                            ↓
         ┌────────────────────────────────┐
         │  PHANTOM COUNT CALCULATION:    │
         │  Server Reported - (TCP - 2)   │
         │  (Subtract 2 monitoring conns) │
         └────────────────────────────────┘
```

---

## Test Comparison

| Feature | 100-Connection Test | 1K-Connection Test |
|---------|--------------------|--------------------|
| **Purpose** | Baseline validation | Scale testing |
| **Connections** | 100 | 1,000 |
| **Ramp Rate** | 20/sec (5s ramp) | 100/sec (10s ramp) |
| **Duration** | 5 minutes | 10 minutes |
| **Metrics Interval** | 5 seconds | 10 seconds (optimized) |
| **Phantom Threshold** | 5 (5%) | 20 (2%) |
| **Heartbeat Interval** | 15 seconds | 30 seconds (reduced load) |
| **Expected Memory** | ~23 MB | ~67 MB |
| **Expected CPU** | 1-5% | 2-10% |
| **Script** | `connection-health-validator.go` | `connection-health-validator-1k.go` |

---

## How to Run the 1K Test

### Step 1: Ensure Clean State
```bash
# Check current connections
curl -s http://34.58.67.124:3004/health | jq '.checks.capacity.current'

# Should show 0 or close to 0
# If not, restart the server
task gcp2:restart:ws-go
```

### Step 2: Start Publisher (if not running)
```bash
task gcp2:publisher:start
```

### Step 3: Run the Test
```bash
task gcp2:test:validate:1k
```

### Step 4: Monitor Output

You'll see output like this every 10 seconds:

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
📊 METRICS @ 18:45:30
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Connection Health:
  TCP Connections:    1002 (ground truth, includes +2 monitoring)
  Client Connections: 1000 (TCP - monitoring)
  Server Reported:    1000
  Phantom Count:      0
  Phantom %:          0.00%
  ✅ Phantom connections within threshold

Message Flow:
  Messages Received:  8,500
  Server PINGs Rcvd:  0
  Heartbeats Sent:    2,000
  Total Errors:       0

Server Resources:
  CPU:                5.23%
  Memory:             67.45 MB
  Goroutines:         2528
  Per-Connection:     49.5 KB
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

### Step 5: Interpret Results

#### ✅ HEALTHY (No Phantoms)
```
Phantom Count:      0
Phantom %:          0.00%
✅ Phantom connections within threshold
```
**Meaning:** Server's connection tracking is accurate. No phantom connections.

#### ⚠️ WARNING (Phantoms Detected)
```
Phantom Count:      45
Phantom %:          4.50%
⚠️  WARNING: Phantom connections exceed threshold! (45 > 20)
```
**Meaning:** Server thinks it has 45 more connections than actually exist. This indicates:
- Connection cleanup is failing
- Dead connections not being detected
- Potential memory/goroutine leak

#### 🔍 INVESTIGATION NEEDED
```
Phantom Count:      200
Phantom %:          20.00%
⚠️  WARNING: Phantom connections exceed threshold! (200 > 20)
```
**Meaning:** Serious connection tracking problem. Check:
- TCP liveness checks working?
- Ping/pong mechanism active?
- Network issues causing timeouts?
- Server overwhelmed?

---

## What to Watch For

### 1. Connection Establishment Phase (First 10-30 seconds)
```
⏫ PHASE 1: Ramping up 1000 connections (100/sec)
⏳ Waiting for all connections to establish...
✅ All 1000 clients connected in 10.2s
📊 Active connections: 1000/1000 (100.0%)
```

**Check:**
- All 1000 connections should establish
- Ramp should complete in ~10 seconds
- No errors during connection

### 2. Stability Phase (Throughout test)
```
📊 METRICS @ 18:47:40
Connection Health:
  TCP Connections:    1002
  Server Reported:    1000
  Phantom Count:      0       ← Should stay 0
  Phantom %:          0.00%   ← Should stay near 0%
```

**Check:**
- Phantom count stays at 0 (or < 20)
- Server reported matches TCP (± monitoring connections)
- CPU/Memory stay stable (not growing)
- Goroutines stay stable (~2528 for 1K connections)

### 3. Message Flow
```
Message Flow:
  Messages Received:  15,240  ← Should grow steadily
  Heartbeats Sent:    4,000   ← ~2 per connection per minute
  Total Errors:       0       ← Should stay 0
```

**Check:**
- Messages continuously received (publisher is working)
- Heartbeats being sent regularly
- Zero or minimal errors

### 4. Resource Usage
```
Server Resources:
  CPU:                5.23%   ← Should be < 10%
  Memory:             67.45 MB ← Should be stable
  Goroutines:         2528    ← Should be stable
  Per-Connection:     49.5 KB ← Should be < 100 KB
```

**Expected Values for 1K:**
- **CPU:** 2-10% (depends on message rate)
- **Memory:** 60-80 MB (baseline 18 MB + 1000 × ~50 KB)
- **Goroutines:** ~2500 (baseline 528 + 1000 × 2)
- **Per-Connection:** 40-60 KB

### 5. Shutdown Phase
```
⬇️  PHASE 3: Graceful shutdown
⏳ Waiting 5 seconds for connections to close...
🔍 Performing final verification...

Post-Shutdown Verification:
TCP Connections:      0 (should be 0)
Server Reported:      0 (should be 0)

✅ CLEAN SHUTDOWN VERIFIED - NO PHANTOM CONNECTIONS
```

**Check:**
- All connections close cleanly
- TCP count returns to 0 (or 2 if monitoring still active)
- Server reported returns to 0
- No phantom connections remain

---

## Troubleshooting

### Issue: TCP count shows -1
```
TCP Connections:    -1 (ground truth)
```

**Cause:** SSH command to check TCP failed  
**Fix:**
1. Check gcloud authentication: `gcloud auth list`
2. Verify instance is running: `task gcp2:list`
3. Test SSH manually: `gcloud compute ssh odin-ws-go --zone=us-central1-a`

### Issue: High error count
```
Total Errors:       250
```

**Possible causes:**
1. Server overloaded (CPU > 75%)
2. Network issues between test machine and server
3. Rate limiting kicking in
4. Server connection limit reached

**Debug:**
```bash
# Check server health
curl http://34.58.67.124:3004/health | jq '.'

# Check server logs
task gcp2:logs:ws-go
```

### Issue: Phantom count growing over time
```
18:45:00 - Phantom Count: 0
18:46:00 - Phantom Count: 5
18:47:00 - Phantom Count: 15
18:48:00 - Phantom Count: 30  ← Growing!
```

**This means:** Connection cleanup is NOT working  
**Action:**
1. Check if TCP liveness checks are enabled
2. Verify ping/pong mechanism is active
3. Check for goroutine leaks: Look at goroutine count growing
4. Review server logs for connection close errors

### Issue: Memory growing continuously
```
18:45:00 - Memory: 67 MB
18:50:00 - Memory: 85 MB
18:55:00 - Memory: 103 MB  ← Memory leak!
```

**This indicates:** Phantom connections causing memory leak  
**Correlation:**
- Check if phantom count is also growing
- Check if goroutines are growing
- This confirms connection cleanup failure

---

## Success Criteria

### ✅ Test PASSES if:
1. **Phantom count stays at 0** (or < 2% threshold)
2. **All 1000 connections establish** successfully
3. **Zero or minimal errors** (< 1%)
4. **CPU stays below 10%** throughout test
5. **Memory stays stable** (60-80 MB range)
6. **Goroutines stay stable** (~2500-2600)
7. **Clean shutdown** with 0 connections remaining

### ❌ Test FAILS if:
1. **Phantom count > 20** (2% threshold)
2. **Phantom count grows over time** (indicates leak)
3. **High error rate** (> 5%)
4. **CPU spikes above 75%** (server overloaded)
5. **Memory grows continuously** (memory leak)
6. **Goroutines grow continuously** (goroutine leak)
7. **Connections remain after shutdown** (cleanup failure)

---

## Advanced: Running on Test-Runner VM

For production-grade testing with minimal latency:

### Step 1: Deploy script to test-runner
```bash
# Copy script to test-runner
gcloud compute scp scripts/connection-health-validator-1k.go \
  odin-test-runner:/tmp/ --zone=us-central1-a
```

### Step 2: SSH and run
```bash
# SSH to test-runner
gcloud compute ssh odin-test-runner --zone=us-central1-a

# Install Go dependencies
cd /tmp
go mod init validator
go get github.com/gorilla/websocket

# Run test (use internal IP for lowest latency)
sed -i 's/34.58.67.124/10.128.0.42/g' connection-health-validator-1k.go
go run connection-health-validator-1k.go
```

**Benefits:**
- Near-zero network latency
- Faster TCP checks (no SSH overhead)
- More realistic load testing

---

## Comparison with Standard Tests

| Test Type | Purpose | Phantom Detection |
|-----------|---------|-------------------|
| `test:light` | Quick throughput test | ❌ No |
| `test:medium` | Medium-scale throughput | ❌ No |
| `test:capacity` | Max connection capacity | ❌ No |
| `test:validate:100` | **Baseline phantom detection** | ✅ Yes (every 5s) |
| `test:validate:1k` | **Scale phantom detection** | ✅ Yes (every 10s) |

**Use validation tests when:**
- You suspect phantom connections
- After deploying connection cleanup fixes
- For capacity planning with accurate metrics
- To validate monitoring accuracy

**Use standard tests when:**
- You need pure throughput metrics
- Testing message delivery performance
- Quick smoke tests
- Load testing without deep analysis

---

## Next Steps After 1K Test

### If Test PASSES ✅
1. Scale to 7K: `task gcp2:test:remote:capacity:7k`
2. Run long soak test (2+ hours)
3. Test with higher message rates
4. Production deployment ready

### If Test FAILS ❌
1. Analyze failure mode (phantoms, CPU, memory)
2. Review server logs: `task gcp2:logs:ws-go`
3. Check connection cleanup code
4. Adjust TCP liveness check parameters
5. Re-test at 100 connections to isolate issue

---

## Metrics to Export

For documentation/reporting:

```bash
# During test, capture metrics
task gcp2:test:validate:1k > test-results-1k.log 2>&1

# Extract key metrics
grep "FINAL METRICS" -A 20 test-results-1k.log
grep "Phantom Count:" test-results-1k.log | tail -20
grep "CPU:" test-results-1k.log | tail -20
grep "Memory:" test-results-1k.log | tail -20
```

---

## Summary

**To test for phantoms at 1K scale:**

```bash
# 1. Clean state
task gcp2:restart:ws-go

# 2. Start publisher
task gcp2:publisher:start

# 3. Run test
task gcp2:test:validate:1k

# 4. Watch for:
#    - Phantom Count: should stay at 0
#    - Phantom %: should stay < 2%
#    - Errors: should stay at 0
#    - Memory: should stabilize ~60-80 MB
#    - CPU: should stay < 10%

# 5. Wait 10 minutes for completion

# 6. Verify clean shutdown:
#    - TCP Connections: 0
#    - Server Reported: 0
```

**Result:** If phantom count stays at 0 throughout the test and clean shutdown succeeds, your connection cleanup is working perfectly at 1K scale! 🎉
