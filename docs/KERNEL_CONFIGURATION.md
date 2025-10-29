# Kernel Configuration for High-Scale WebSocket Connections

**Purpose**: This document explains the kernel-level tuning required to support 15,000+ concurrent WebSocket connections on GCP e2-standard-4 instances.

**Last Updated**: 2025-10-28  
**Target Capacity**: 15,000 connections (ws-go server), 15,000 clients (test-runner)

---

## Table of Contents

1. [Overview](#overview)
2. [Server Configuration (ws-go)](#server-configuration-ws-go)
3. [Client Configuration (test-runner)](#client-configuration-test-runner)
4. [Why These Values?](#why-these-values)
5. [Verification](#verification)
6. [Troubleshooting](#troubleshooting)

---

## Overview

### The Problem

Default Linux kernel parameters are tuned for **general-purpose workloads** (web servers, databases), not high-connection WebSocket services. At 15K connections, you hit multiple hard limits:

| Limit | Default | At 15K Connections | Result |
|-------|---------|-------------------|--------|
| `nf_conntrack_max` | 65,536 | 60,000 entries (92%) | Connections dropped |
| `somaxconn` | 4,096 | 100 conn/sec burst | SYN drops |
| Ephemeral ports | 28,231 | 15K + TIME_WAIT | Port exhaustion |
| File descriptors | 65,536 | 15K + overhead | FD exhaustion |

### The Solution

**Production kernel tuning** configured during VM creation (in `create:*` tasks):

- ✅ Server-side: Handle 15K inbound connections through Docker NAT
- ✅ Client-side: Establish 15K outbound connections without port exhaustion
- ✅ Persistent: Survives reboots, disaster recovery, VM recreation

---

## Server Configuration (ws-go)

### Applied in: `taskfiles/isolated-setup.yml` → `create:ws-go` startup script

### 1. Docker NAT Connection Tracking

**Problem**: Each WebSocket connection through Docker NAT consumes **4 conntrack entries**:
```
Client → GCP Load Balancer → Host Port 3004 → Docker NAT → Container Port 3002
```

**Configuration**:
```bash
sysctl -w net.netfilter.nf_conntrack_max=262144
```

**Rationale**:
- Default: 65,536 entries
- **Production: 262,144 entries** (4x capacity)
- Formula: `15,000 connections × 4 entries = 60,000 entries (23% usage)`
- Headroom: 77% free for connection spikes, monitoring, SSH

**Critical**: Without this, at 12K connections (73% full), kernel **randomly drops ESTABLISHED connections** causing the "gaps" you saw in Grafana.

---

### 2. TCP Listen Queue

**Problem**: 100 conn/sec ramp rate creates **SYN packet bursts** that overflow default queue.

**Configuration**:
```bash
sysctl -w net.ipv4.tcp_max_syn_backlog=16384
sysctl -w net.core.somaxconn=16384
```

**Rationale**:
- Default: 4,096 slots
- **Production: 16,384 slots** (4x capacity)
- Formula: `100 conn/sec × 10 sec burst window = 1,000 slots (6% usage)`
- Industry standard: Nginx, HAProxy use 16K-32K for production

**Impact**: Prevents timeout errors during connection ramp-up phase.

---

### 3. SYN Cookie Protection

**Configuration**:
```bash
sysctl -w net.ipv4.tcp_syncookies=1
```

**Rationale**:
- Enables SYN flood protection (RFC 4987)
- When SYN backlog fills, kernel generates stateless cookies
- Prevents DoS attacks from exhausting connection queue
- **No performance cost** - only activates under attack

---

### 4. File Descriptor Limits (ulimit)

**Problem**: Default 65K FDs insufficient for 15K connections + Docker + monitoring.

**Configuration**:
```bash
# /etc/security/limits.conf
* soft nofile 200000
* hard nofile 200000
deploy soft nofile 200000
deploy hard nofile 200000

# /etc/docker/daemon.json
{
  "default-ulimits": {
    "nofile": {"Hard": 200000, "Soft": 200000}
  }
}
```

**Rationale**:
- Each connection: 1 socket FD
- Docker container: ~500 FDs
- Promtail (logging): ~100 FDs
- System overhead: ~1,000 FDs
- **Total: ~17,000 FDs** (9% of 200K limit)

---

## Client Configuration (test-runner)

### Applied in: `taskfiles/isolated-setup.yml` → `create:test-runner` startup script

### 1. Ephemeral Port Range (CRITICAL)

**Problem**: Each outbound connection needs a unique source port. Default range insufficient for 15K connections.

**Configuration**:
```bash
sysctl -w net.ipv4.ip_local_port_range="10000 65000"
```

**Rationale**:
- Default: 32768-60999 (28,231 ports)
- **Production: 10000-65000 (55,000 ports)**
- Why start at 10000? Reserve 1024-10000 for server applications (SSH, monitoring)
- Formula: `15,000 active + 15,000 TIME_WAIT = 30,000 ports needed (55% usage)`

**Critical**: At 28K port limit, you hit exhaustion at ~12K connections, causing "i/o timeout" errors.

---

### 2. TIME_WAIT Socket Reuse

**Configuration**:
```bash
sysctl -w net.ipv4.tcp_tw_reuse=1
```

**Rationale**:
- Allows reusing ports in TIME_WAIT state for **outbound connections**
- Safe for clients (test-runner), **unsafe for servers** (can cause data corruption)
- Reduces port exhaustion during rapid test cycles
- TIME_WAIT period: 60 seconds (RFC 793 standard)

---

### 3. TIME_WAIT Timeout Reduction

**Configuration**:
```bash
sysctl -w net.ipv4.tcp_fin_timeout=15
```

**Rationale**:
- Default: 60 seconds
- **Production: 15 seconds** (4x faster cleanup)
- Frees ephemeral ports faster between test runs
- Safe for internal test environment (LAN, low latency)
- **Do NOT use on WAN-facing servers** (can cause connection resets)

---

### 4. Connection Tracking Table

**Configuration**:
```bash
sysctl -w net.netfilter.nf_conntrack_max=200000
```

**Rationale**:
- Test-runner may run tests inside Docker containers
- Each outbound connection: ~2 conntrack entries (less than server's 4)
- Formula: `15,000 connections × 2 = 30,000 entries (15% usage)`

---

### 5. File Descriptor Limits

Same as server configuration (200K limit).

---

## Why These Values?

### Capacity Planning Formula

**Server (ws-go):**
```
Conntrack entries = connections × 4 (Docker NAT overhead)
Required limit = (connections × 4) / 0.75  (75% soft limit)
15,000 × 4 / 0.75 = 80,000 → rounded to 262,144 (next power of 2)
```

**Client (test-runner):**
```
Ports needed = active_connections + TIME_WAIT_connections
TIME_WAIT = active × (TIME_WAIT_seconds / connection_lifetime)
15,000 + (15,000 × 60 / 60) = 30,000 ports
Add 80% headroom: 30,000 × 1.8 = 54,000 → 55,000 available
```

### Industry Standards

| Platform | Conntrack | SYN Backlog | Ephemeral Ports |
|----------|-----------|-------------|-----------------|
| **Nginx** | 256K-1M | 16K-32K | - |
| **HAProxy** | 512K | 16K | - |
| **Kubernetes** | 524K | 16K | 32K-60K |
| **AWS ELB** | 1M | 32K | - |
| **Our Config** | **262K** | **16K** | **55K** |

Our values align with production-grade infrastructure.

---

## Verification

### On ws-go Server

```bash
# SSH into server
gcloud compute ssh odin-ws-go --zone=us-central1-a

# Check conntrack
cat /proc/sys/net/netfilter/nf_conntrack_max
# Expected: 262144

cat /proc/sys/net/netfilter/nf_conntrack_count
# Expected: <60000 during 15K connection test (23% usage)

# Check TCP queues
sysctl net.core.somaxconn
# Expected: 16384

sysctl net.ipv4.tcp_max_syn_backlog
# Expected: 16384

# Check ulimit
ulimit -n
# Expected: 200000

# Check Docker ulimit
docker run --rm ubuntu:22.04 bash -c 'ulimit -n'
# Expected: 200000
```

### On test-runner

```bash
# SSH into test-runner
gcloud compute ssh odin-test-runner --zone=us-central1-a

# Check ephemeral port range
sysctl net.ipv4.ip_local_port_range
# Expected: 10000 65000

# Check TIME_WAIT settings
sysctl net.ipv4.tcp_tw_reuse
# Expected: 1

sysctl net.ipv4.tcp_fin_timeout
# Expected: 15

# Check port usage during test
ss -tan | grep ESTAB | wc -l      # Active connections
ss -tan | grep TIME-WAIT | wc -l  # Waiting ports
# Expected: <30000 total (55% of 55K capacity)
```

---

## Troubleshooting

### Problem: Connections dropped at 12K (not reaching 15K)

**Symptom**: Grafana shows periodic gaps, Prometheus reports fewer connections than test-runner

**Diagnosis**:
```bash
# Check conntrack usage
gcloud compute ssh odin-ws-go --zone=us-central1-a \
  --command="cat /proc/sys/net/netfilter/nf_conntrack_count"

# If >48000 (>73%), conntrack is full
```

**Root Cause**: `nf_conntrack_max` too low (likely still 65K default)

**Fix**:
1. VMs need to be **recreated** with new startup script
2. Follow: `docs/VM_RECREATION_GUIDE.md`

---

### Problem: "dial failed: i/o timeout" errors on test-runner

**Symptom**: Test logs show TCP timeout errors during connection ramp

**Diagnosis**:
```bash
# Check ephemeral port usage
gcloud compute ssh odin-test-runner --zone=us-central1-a \
  --command="cat /proc/sys/net/ipv4/ip_local_port_range"

# If still "32768 60999", port range not updated
```

**Root Cause**: Ephemeral port range not expanded to 10K-65K

**Fix**: Recreate test-runner VM with new startup script

---

### Problem: Cannot create new connections after reboot

**Symptom**: After VM reboot, can only establish ~4K connections

**Diagnosis**:
```bash
# Check if sysctl settings persisted
sysctl net.netfilter.nf_conntrack_max
# If 65536, settings didn't persist
```

**Root Cause**: `/etc/sysctl.conf` not updated (manual sysctl commands don't persist)

**Fix**: 
- Settings must be in VM startup script (create:* tasks)
- **DO NOT** add to deploy:* tasks (won't survive reboot)
- Recreate VMs with proper startup scripts

---

### Problem: Docker containers still hitting FD limits

**Symptom**: Container logs show "too many open files" errors

**Diagnosis**:
```bash
# Check Docker daemon config
gcloud compute ssh odin-ws-go --zone=us-central1-a \
  --command="cat /etc/docker/daemon.json"

# Should contain default-ulimits
```

**Root Cause**: `/etc/docker/daemon.json` not configured or Docker not restarted

**Fix**: Recreate VM (startup script configures Docker before first use)

---

## Configuration Lifecycle

### Where Configs Are Applied

| Layer | Location | Applied When | Persists? |
|-------|----------|--------------|-----------|
| **Kernel params** | `create:*` startup script | VM creation | ✅ YES (via /etc/sysctl.conf) |
| **ulimit** | `create:*` startup script | VM creation | ✅ YES (via /etc/security/limits.conf) |
| **Docker ulimit** | `create:*` startup script | VM creation | ✅ YES (via /etc/docker/daemon.json) |
| ❌ **deploy:* tasks** | ❌ WRONG PLACE | Application deploy | ❌ NO (lost on reboot) |

### Best Practice

✅ **Infrastructure config** (kernel, limits) → `create:*` tasks (VM startup script)  
✅ **Application config** (env vars, code) → `deploy:*` tasks  

**Golden Rule**: If it's required for the VM to **physically support** 15K connections, it belongs in `create:*`.

---

## Related Documentation

- [VM_RECREATION_GUIDE.md](./VM_RECREATION_GUIDE.md) - Step-by-step VM recreation process
- [CAPACITY_PLANNING.md](./CAPACITY_PLANNING.md) - Connection capacity analysis
- [PHANTOM_DETECTION_TESTING_GUIDE.md](./PHANTOM_DETECTION_TESTING_GUIDE.md) - Testing phantom connection cleanup

---

## References

- [Linux kernel sysctl documentation](https://www.kernel.org/doc/Documentation/networking/ip-sysctl.txt)
- [Docker daemon configuration](https://docs.docker.com/engine/reference/commandline/dockerd/#daemon-configuration-file)
- [RFC 4987: TCP SYN Flooding Attacks](https://datatracker.ietf.org/doc/html/rfc4987)
- [Netfilter conntrack tuning](https://wiki.khnet.info/index.php/Conntrack_tuning)
