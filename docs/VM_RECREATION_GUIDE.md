# VM Recreation Guide - Kernel Configuration Migration

**Purpose**: Step-by-step guide to recreate VMs with production kernel configuration for 15K+ connection support.

**Estimated Duration**: 15 minutes total (5 min ws-go + 5 min test-runner + 5 min verification)  
**Downtime**: 5 minutes for ws-go (active connections will drop)  
**Risk Level**: LOW (can rollback by recreating with old config)

---

## Table of Contents

1. [When to Recreate VMs](#when-to-recreate-vms)
2. [Pre-Migration Checklist](#pre-migration-checklist)
3. [Recreation Process](#recreation-process)
4. [Verification](#verification)
5. [Rollback Plan](#rollback-plan)
6. [Post-Migration Testing](#post-migration-testing)

---

## When to Recreate VMs

### You MUST recreate if:

✅ VMs created **before** kernel config changes (2025-10-28)  
✅ Testing at 15K connections hits capacity limits (drops at 12K)  
✅ Seeing "i/o timeout" errors from test-runner  
✅ Conntrack usage >70% (`/proc/sys/net/netfilter/nf_conntrack_count`)

### You can skip recreation if:

❌ VMs created **after** 2025-10-28 with updated `create:*` tasks  
❌ Current capacity targets <10K connections  
❌ Only doing light testing (<5K connections)

### Check if recreation needed:

```bash
# Check ws-go conntrack limit
gcloud compute ssh odin-ws-go --zone=us-central1-a \
  --command="sysctl net.netfilter.nf_conntrack_max"

# Expected: 262144 (NEW) vs 65536 (OLD - needs recreation)

# Check test-runner port range
gcloud compute ssh odin-test-runner --zone=us-central1-a \
  --command="sysctl net.ipv4.ip_local_port_range"

# Expected: "10000 65000" (NEW) vs "32768 60999" (OLD - needs recreation)
```

---

## Pre-Migration Checklist

### 1. Schedule Maintenance Window

**Recommended time**: Low-traffic period (weekends, late night)  
**Notify**: Stakeholders, monitoring teams  
**Duration**: 15 minutes

### 2. Backup Current State (Optional but Recommended)

```bash
# Save current VM metadata
gcloud compute instances describe odin-ws-go --zone=us-central1-a \
  > /tmp/ws-go-backup-metadata.yaml

gcloud compute instances describe odin-test-runner --zone=us-central1-a \
  > /tmp/test-runner-backup-metadata.yaml

# Note: We don't back up disks as VMs are stateless (containers pulled from registry)
```

### 3. Verify Latest Code

```bash
# Ensure you have latest taskfile with kernel configs
cd /Volumes/Dev/Codev/Toniq/ws_poc

# Check create:ws-go has new config
grep "net.netfilter.nf_conntrack_max=262144" taskfiles/isolated-setup.yml
# Should find match at line ~166

# Check create:test-runner has new config
grep "net.ipv4.ip_local_port_range.*10000 65000" taskfiles/isolated-setup.yml
# Should find match at line ~300
```

### 4. Confirm Variables Set

```bash
# Check environment variables (from .env or shell)
echo $WS_GO_INSTANCE         # Expected: odin-ws-go
echo $TEST_RUNNER_INSTANCE   # Expected: odin-test-runner
echo $GCP_ZONE              # Expected: us-central1-a
echo $BACKEND_INTERNAL_IP   # Expected: 10.128.0.2 (or your backend IP)
```

---

## Recreation Process

### Phase 1: Recreate ws-go (5 min downtime)

**Impact**: Active WebSocket connections will drop

#### Step 1.1: Stop Current ws-go

```bash
# Stop containers gracefully
gcloud compute ssh odin-ws-go --zone=us-central1-a \
  --command="sudo docker compose -f /home/deploy/ws_poc/docker-compose.yml down"

# Wait for graceful shutdown
sleep 10
```

#### Step 1.2: Delete Old VM

```bash
# Delete VM (keeps persistent disk settings, but we're using ephemeral disk)
task gcp2:delete:ws-go

# Or manually:
gcloud compute instances delete odin-ws-go --zone=us-central1-a --quiet
```

#### Step 1.3: Create New VM with Kernel Config

```bash
# This creates VM with new startup script containing kernel tuning
task gcp2:create:ws-go

# Startup script will:
# - Install Docker
# - Configure kernel (conntrack, TCP queues, ulimit)
# - Persist settings to /etc/sysctl.conf
# - Configure Docker daemon ulimits
```

**Expected output**:
```
🖥️  Creating ws-go instance...
Created [https://www.googleapis.com/compute/v1/projects/.../instances/odin-ws-go]
⏳ Waiting for instance to be ready...
✅ ws-go instance created
```

#### Step 1.4: Deploy Application

```bash
# Deploy ws-go application (containers, config)
task gcp2:deploy:ws-go

# This will:
# - Copy source code to VM
# - Substitute environment variables
# - Build Docker images
# - Start containers
# - Health check
```

**Expected output**:
```
📦 Deploying ws-go...
🚀 Building and starting ws-go...
⏳ Waiting for ws-go to start...
🏥 Checking ws-go health...
✅ ws-go deployed
```

#### Step 1.5: Verify ws-go

```bash
# Check health
task gcp2:health:ws-go

# Verify kernel settings applied
gcloud compute ssh odin-ws-go --zone=us-central1-a --command="
  echo 'Conntrack max:' && sysctl net.netfilter.nf_conntrack_max
  echo 'SYN backlog:' && sysctl net.ipv4.tcp_max_syn_backlog
  echo 'somaxconn:' && sysctl net.core.somaxconn
  echo 'ulimit:' && ulimit -n
"

# Expected:
# Conntrack max: 262144
# SYN backlog: 16384
# somaxconn: 16384
# ulimit: 200000
```

---

### Phase 2: Recreate test-runner (No downtime)

**Impact**: None (test-runner is not serving production traffic)

#### Step 2.1: Stop Any Running Tests

```bash
# Kill any active tests
gcloud compute ssh odin-test-runner --zone=us-central1-a \
  --command="sudo pkill -9 sustained-load || true"
```

#### Step 2.2: Delete Old VM

```bash
task gcp2:delete:test-runner

# Or manually:
gcloud compute instances delete odin-test-runner --zone=us-central1-a --quiet
```

#### Step 2.3: Create New VM

```bash
# Creates VM with kernel tuning for client-side connections
task gcp2:create:test-runner

# Startup script will:
# - Install Docker
# - Configure ephemeral ports (10K-65K)
# - Enable TIME_WAIT reuse
# - Configure ulimit
```

#### Step 2.4: Deploy Test Runner

```bash
# Deploy Go test runner
task gcp2:deploy:test-runner-go
```

#### Step 2.5: Verify test-runner

```bash
# Verify kernel settings
gcloud compute ssh odin-test-runner --zone=us-central1-a --command="
  echo 'Ephemeral ports:' && sysctl net.ipv4.ip_local_port_range
  echo 'TIME_WAIT reuse:' && sysctl net.ipv4.tcp_tw_reuse
  echo 'FIN timeout:' && sysctl net.ipv4.tcp_fin_timeout
  echo 'Conntrack:' && sysctl net.netfilter.nf_conntrack_max
  echo 'ulimit:' && ulimit -n
"

# Expected:
# Ephemeral ports: 10000 65000
# TIME_WAIT reuse: 1
# FIN timeout: 15
# Conntrack: 200000
# ulimit: 200000
```

---

## Verification

### Test 1: Confirm Settings Persist Across Reboot

```bash
# Reboot ws-go
gcloud compute instances stop odin-ws-go --zone=us-central1-a
sleep 10
gcloud compute instances start odin-ws-go --zone=us-central1-a
sleep 30

# Check settings still applied
gcloud compute ssh odin-ws-go --zone=us-central1-a \
  --command="sysctl net.netfilter.nf_conntrack_max"

# Expected: 262144 (if 65536, settings didn't persist - BAD!)
```

### Test 2: Small-Scale Connection Test (100 connections)

```bash
# Restart services
task gcp2:restart:clean

# Run small test
task gcp2:test:validate:100

# Expected: 0 phantoms, 100% success rate
```

### Test 3: Medium-Scale Test (5K connections)

```bash
# Run medium test
task gcp2:test:remote:medium

# Expected:
# - 5K connections established
# - <0.5% failure rate
# - No gaps in Grafana
# - Conntrack usage <10%
```

### Test 4: Full-Scale Test (15K connections)

```bash
# Clean slate
task gcp2:restart:clean
sleep 120  # Wait for TIME_WAIT cleanup

# Run 15K test
task gcp2:test:remote:capacity

# Expected:
# - 15K connections reached (not stopping at 12K!)
# - <1% failure rate
# - No gaps in Grafana
# - Conntrack usage ~23% (60K/262K)
# - Port usage ~55% (30K/55K)
```

---

## Rollback Plan

### If things go wrong during migration:

#### Option A: Recreate with Old Config (Temporary)

```bash
# Manually create VM with old values (emergency only)
gcloud compute instances create odin-ws-go \
  --zone=us-central1-a \
  --machine-type=e2-standard-4 \
  --network=default \
  --subnet=default \
  --tags=ws-server \
  --boot-disk-size=10GB \
  --boot-disk-type=pd-standard \
  --image-family=ubuntu-2404-lts-amd64 \
  --image-project=ubuntu-os-cloud \
  --metadata=startup-script='#!/bin/bash
    apt-get update && apt-get install -y curl git
    curl -fsSL https://get.docker.com -o get-docker.sh && sh get-docker.sh
    curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
    chmod +x /usr/local/bin/docker-compose
    sysctl -w net.ipv4.tcp_max_syn_backlog=8192
    sysctl -w net.core.somaxconn=8192
    useradd -m -s /bin/bash deploy
    usermod -aG docker deploy
  '

# Then deploy
task gcp2:deploy:ws-go
```

#### Option B: Restore from Backup Metadata

```bash
# If you saved metadata earlier
gcloud compute instances create odin-ws-go \
  --source-instance-template=/tmp/ws-go-backup-metadata.yaml \
  --zone=us-central1-a

# Then deploy
task gcp2:deploy:ws-go
```

#### Option C: Investigate and Fix

Most common issues:

1. **Syntax error in startup script**: Check logs
   ```bash
   gcloud compute instances get-serial-port-output odin-ws-go --zone=us-central1-a
   ```

2. **Settings not persisting**: Check /etc/sysctl.conf
   ```bash
   gcloud compute ssh odin-ws-go --zone=us-central1-a \
     --command="cat /etc/sysctl.conf"
   ```

3. **Docker not starting**: Check Docker logs
   ```bash
   gcloud compute ssh odin-ws-go --zone=us-central1-a \
     --command="sudo systemctl status docker"
   ```

---

## Post-Migration Testing

### Recommended Test Sequence

1. ✅ **100 connections** - Verify basics work
2. ✅ **1K connections** - Test mid-scale stability
3. ✅ **5K connections** - Confirm no early bottlenecks
4. ✅ **12K connections** - Test OLD limit threshold (should pass now!)
5. ✅ **15K connections** - Full production capacity test

### Success Criteria

| Metric | Target | How to Verify |
|--------|--------|---------------|
| **Connection success rate** | >99% | Test runner logs |
| **Phantom connections** | 0 | Validator script |
| **Grafana gaps** | None | Visual inspection |
| **Conntrack usage** | <25% | `nf_conntrack_count` |
| **Port exhaustion** | <60% | `ss -tan \| wc -l` |
| **CPU usage** | <40% | Prometheus/Grafana |
| **Memory usage** | <65% | Prometheus/Grafana |

### Continuous Monitoring (First 24 Hours)

```bash
# Set up monitoring alert (Grafana)
# Alert if:
# - Connections drop >5% in 1 minute
# - Conntrack usage >80%
# - CPU >70% sustained
# - Memory >85%
```

---

## FAQ

### Q: Do I need to recreate backend VM?

**A**: No. Backend (NATS, Prometheus, Grafana) doesn't handle WebSocket connections, so kernel limits are sufficient.

### Q: Will recreating VMs change their internal IPs?

**A**: Possibly yes. Check and update:
```bash
# Get new IPs
gcloud compute instances describe odin-ws-go --zone=us-central1-a \
  --format='get(networkInterfaces[0].networkIP)'

# Update .env if changed
# Redeploy backend to update Prometheus targets
task gcp2:deploy:backend
```

### Q: Can I apply kernel settings without recreating VMs?

**A**: Yes temporarily, but they won't persist across reboots:
```bash
# TEMPORARY FIX (lost on reboot)
gcloud compute ssh odin-ws-go --zone=us-central1-a --command="
  sudo sysctl -w net.netfilter.nf_conntrack_max=262144
  sudo sysctl -w net.core.somaxconn=16384
  # ... etc
"
```

**Proper fix**: Add to `/etc/sysctl.conf` and recreate Docker daemon config. Easier to just recreate VM.

### Q: What if I can't schedule downtime for ws-go?

**A**: Blue-green deployment:
1. Create new ws-go VM with different name (`odin-ws-go-v2`)
2. Deploy application to new VM
3. Update load balancer to point to new VM
4. Test thoroughly
5. Delete old VM

### Q: How do I verify settings are in startup script?

```bash
# View startup script metadata
gcloud compute instances describe odin-ws-go --zone=us-central1-a \
  --format='value(metadata.items[startup-script])'

# Should see: net.netfilter.nf_conntrack_max=262144
```

---

## Timeline Summary

| Phase | Duration | Downtime | Can Run in Parallel? |
|-------|----------|----------|---------------------|
| Pre-checks | 5 min | None | - |
| Recreate ws-go | 5 min | YES (5 min) | No |
| Deploy ws-go | 3 min | None | No |
| Verify ws-go | 2 min | None | No |
| Recreate test-runner | 2 min | None | Yes (after ws-go) |
| Deploy test-runner | 2 min | None | Yes |
| Verify test-runner | 1 min | None | Yes |
| **Total** | **15 min** | **5 min** | - |

---

## Related Documentation

- [KERNEL_CONFIGURATION.md](./KERNEL_CONFIGURATION.md) - Detailed kernel parameter explanation
- [CAPACITY_PLANNING.md](./CAPACITY_PLANNING.md) - Why 15K is the target
- [PHANTOM_DETECTION_TESTING_GUIDE.md](./PHANTOM_DETECTION_TESTING_GUIDE.md) - Post-recreation testing

---

## Checklist

Print this checklist and check off each step:

```
PRE-MIGRATION:
[ ] Scheduled maintenance window
[ ] Notified stakeholders
[ ] Backed up VM metadata
[ ] Verified latest taskfile has kernel configs
[ ] Confirmed environment variables set

WS-GO RECREATION:
[ ] Stopped current ws-go containers
[ ] Deleted old ws-go VM
[ ] Created new ws-go VM (task gcp2:create:ws-go)
[ ] Deployed application (task gcp2:deploy:ws-go)
[ ] Verified kernel settings (conntrack=262144)
[ ] Health check passed

TEST-RUNNER RECREATION:
[ ] Stopped running tests
[ ] Deleted old test-runner VM
[ ] Created new test-runner VM (task gcp2:create:test-runner)
[ ] Deployed test runner (task gcp2:deploy:test-runner-go)
[ ] Verified kernel settings (ports=10000-65000)

VERIFICATION:
[ ] Reboot test passed (settings persist)
[ ] 100-connection test: 0 phantoms
[ ] 5K-connection test: <0.5% failure
[ ] 15K-connection test: SUCCESS (reached 15K!)
[ ] No gaps in Grafana
[ ] Conntrack <25%, ports <60%
[ ] Updated monitoring/documentation

POST-MIGRATION:
[ ] Monitoring alerts configured
[ ] Team notified of successful migration
[ ] Documentation updated with new IPs (if changed)
[ ] Old VM backups can be deleted (after 7 days)
```

---

**Last Updated**: 2025-10-28  
**Next Review**: After first production 15K connection test
