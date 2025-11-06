# Local Deployment Plan - Mirror GCP Distributed Setup

## Overview
Create a local deployment that mirrors the GCP distributed setup as closely as possible, combining the two-instance architecture into a single docker-compose file for local development.

## Current GCP Architecture

### Instance 1: Backend (odin-backend - e2-small)
- **Redpanda** (Kafka/streaming) - 4GB, 2 CPUs
- **Redpanda Console** (UI) - 256MB, 0.2 CPUs
- **Publisher** (Event generator) - 256MB, 0.3 CPUs
- **Prometheus** (Metrics) - 256MB, 0.3 CPUs
- **Grafana** (Dashboards) - 512MB, 0.2 CPUs
- **Loki** (Logs) - 256MB, 0.2 CPUs
- **Promtail** (Log shipper) - 128MB, 0.1 CPUs

**Total Resources**: ~5.5GB RAM, ~3.3 CPUs

### Instance 2: WS Server (odin-ws-go - e2-standard-4)
- **WS Server** (Go WebSocket) - 14.5GB, 1 CPU
- **Promtail** (Log shipper) - 512MB, 1 CPU

**Total Resources**: ~15GB RAM, ~2 CPUs

**Combined Total**: ~20.5GB RAM, ~5.3 CPUs

## Local Deployment Strategy

### 1. Architecture
**Single Docker Compose**: Merge both instances into one file
- All 9 services in one `docker-compose.local.yml`
- Shared Docker network (no need for internal IPs)
- Services communicate via container names
- Port mappings for localhost access

### 2. Resource Adjustments
**Local Development Settings** (assuming 16-32GB dev machine):
- Reduce memory limits for development
- Keep CPU limits low to prevent fan noise
- Maintain service ratios for realistic testing

**Proposed Resource Limits**:
```yaml
Redpanda:      2GB  (down from 4GB)  - 1 CPU
Console:       256MB (same)          - 0.2 CPU
Publisher:     256MB (same)          - 0.3 CPU
WS Server:     4GB   (down from 14.5GB) - 1 CPU
Prometheus:    256MB (same)          - 0.3 CPU
Grafana:       512MB (same)          - 0.2 CPU
Loki:          256MB (same)          - 0.2 CPU
Promtail (x2): 256MB (down from 512+128MB) - 0.2 CPU
---
Total:         ~8GB RAM, ~3.7 CPUs
```

### 3. Configuration Changes

#### Redpanda/Kafka
- **GCP**: `NATS_URL=nats://${BACKEND_INTERNAL_IP}:4222`
- **Local**: `KAFKA_BROKERS=redpanda:9092`

#### WS Server
- **GCP**: External IP access, separate instance
- **Local**: Same docker network, container name

#### Networking
- **GCP**: Internal IPs (10.128.0.x), firewall rules
- **Local**: Docker bridge network, all services accessible

#### Ports (localhost access)
```
Redpanda Console:  8080
Grafana:           3010
Prometheus:        9091
Loki:              3100
Publisher API:     3003
WS Server:         3004
Redpanda Admin:    9644
```

### 4. Environment Configuration

#### New File: `deployments/local/.env.local`
```bash
# Environment
ENVIRONMENT=local

# WS Server
WS_ADDR=:3002
KAFKA_BROKERS=redpanda:9092
KAFKA_CONSUMER_GROUP=ws-server-local

# Resource Limits (local dev)
WS_CPU_LIMIT=1.0
WS_MEMORY_LIMIT=4294967296  # 4GB

# Capacity (reduced for local)
WS_MAX_CONNECTIONS=1000  # Down from 12K

# Worker Pool
WS_WORKER_POOL_SIZE=32   # Auto-calculated: 1.0 * 2 = 2, rounded to 32
WS_WORKER_QUEUE_SIZE=3200

# Rate Limiting
WS_MAX_KAFKA_RATE=1000
WS_MAX_BROADCAST_RATE=25
WS_MAX_GOROUTINES=3000  # Scaled down from 30K

# Safety Thresholds
WS_CPU_REJECT_THRESHOLD=75.0
WS_CPU_PAUSE_THRESHOLD=80.0

# Monitoring
METRICS_INTERVAL=15s

# Logging
LOG_LEVEL=debug  # More verbose for local dev
LOG_FORMAT=pretty  # Human-readable for terminal
```

### 5. File Structure

```
deployments/
├── gcp-distributed/           # Production (unchanged)
│   ├── backend/
│   │   ├── docker-compose.yml
│   │   ├── prometheus.yml
│   │   └── grafana/
│   └── ws-server/
│       ├── docker-compose.yml
│       ├── .env.production
│       └── promtail-config.yml
│
└── local/                     # New local deployment
    ├── docker-compose.yml     # Combined all services
    ├── .env.local             # Local configuration
    ├── .env.local.example     # Template for developers
    ├── prometheus.yml         # Copy from gcp-distributed/backend
    ├── loki-config.yml        # Copy from gcp-distributed/backend
    ├── promtail-config.yml    # Adapted for local
    ├── grafana/               # Copy from gcp-distributed/backend
    │   └── provisioning/
    └── README.md              # Local setup instructions
```

## Implementation Steps

### Phase 1: Create Base Structure ✅
1. ✅ Create `deployments/local/` directory
2. ✅ Copy configuration files from `gcp-distributed/backend/`
3. ✅ Create `.env.local.example` template
4. ✅ Create base `docker-compose.yml` structure

### Phase 2: Merge Services
1. **Backend Services** (from gcp-distributed/backend):
   - Copy Redpanda service definition
   - Copy Redpanda Console service
   - Copy Publisher service
   - Copy Prometheus service
   - Copy Grafana service
   - Copy Loki service
   - Copy Promtail service

2. **WS Server Service** (from gcp-distributed/ws-server):
   - Copy WS Server service definition
   - Update build context path
   - Update environment variables (NATS → Kafka)
   - Adjust resource limits

### Phase 3: Update Networking
1. **Remove External Dependencies**:
   - Remove `${BACKEND_INTERNAL_IP}` references
   - Use container names for service discovery

2. **Docker Network**:
   - Create named network: `odin-local`
   - Attach all services to network
   - Enable service name DNS resolution

3. **Port Bindings**:
   - Map all necessary ports to localhost
   - Use 0.0.0.0 for accessibility

### Phase 4: Configuration Adaptation

#### Prometheus
- Update scrape targets:
  - `ws-go:3002` → `ws-server:3002`
  - Keep same job names for dashboard compatibility

#### Promtail
- **Local approach**: Single Promtail for all containers
- Remove separate ws-server Promtail
- Update config to scrape all containers

#### Loki
- Keep configuration same
- Accessible at `http://loki:3100`

#### Grafana
- Keep provisioning unchanged
- Datasources will use container names
- Dashboards work without modification

### Phase 5: WS Server Updates
1. **Dockerfile Context**:
   - Update build context: `../../../ws` (relative to local/)
   - Keep Dockerfile unchanged

2. **Environment Variables**:
   - Replace `NATS_URL` with `KAFKA_BROKERS`
   - Remove `JS_*` JetStream variables
   - Add `KAFKA_CONSUMER_GROUP`

3. **Health Check** (optional):
   - Add healthcheck for WS server
   - Check `/health` endpoint

### Phase 6: Publisher Updates
1. **Build Context**:
   - Update to `../../../publisher`
   
2. **Environment**:
   - Update `KAFKA_BROKERS=redpanda:9092`
   - Keep API port 3003

### Phase 7: Resource Optimization
1. **Set Appropriate Limits**:
   - Balance between realism and laptop resources
   - Allow override via environment variables
   
2. **Scaling Options**:
   - Document how to scale down for low-end machines
   - Document how to scale up for testing

### Phase 8: Documentation
1. **README.md** in `deployments/local/`:
   - Prerequisites (Docker, Docker Compose, resources)
   - Quick start guide
   - Configuration options
   - Troubleshooting
   - Differences from GCP deployment

2. **Setup Script** (optional):
   - `setup-local.sh` - One-command setup
   - Create topics in Redpanda
   - Start all services
   - Health checks

### Phase 9: Testing
1. **Service Startup**:
   - All containers start successfully
   - Health checks pass
   - No resource exhaustion

2. **Connectivity**:
   - WS Server connects to Redpanda
   - Publisher publishes to Redpanda
   - WS Server consumes messages
   - Logs flow to Loki
   - Metrics scraped by Prometheus

3. **Monitoring**:
   - Grafana dashboards show data
   - All datasources connected
   - No errors in logs

4. **WebSocket Connections**:
   - Connect test client to `ws://localhost:3004/ws`
   - Subscribe to tokens
   - Verify message reception

## Key Differences from GCP

### What's the Same
✅ **All services present** - Full monitoring stack
✅ **Service architecture** - Same relationships and dependencies
✅ **Configuration structure** - Similar env vars and files
✅ **Monitoring stack** - Prometheus + Grafana + Loki
✅ **Port consistency** - Same internal ports

### What's Different
⚠️ **Resource limits** - Reduced for local development
⚠️ **Connection capacity** - 1K vs 12K (for laptop testing)
⚠️ **Networking** - Docker network vs GCP internal IPs
⚠️ **Log format** - Pretty vs JSON (for readability)
⚠️ **Log level** - Debug vs Info (more verbose)
⚠️ **Single compose** - One file vs two instances

### What's Removed
❌ **Promtail duplication** - Single Promtail instead of two
❌ **External IP dependencies** - No ${BACKEND_INTERNAL_IP}
❌ **NATS** - Replaced with Kafka/Redpanda
❌ **JetStream config** - No longer needed

## Benefits of Local Setup

### Development
- **Fast iteration**: No deployment delay
- **Easy debugging**: All logs in one place
- **Cost-free testing**: No GCP charges
- **Offline capable**: Works without internet

### Testing
- **Full integration**: All services together
- **Realistic**: Same architecture as production
- **Observable**: Full monitoring stack
- **Debuggable**: Debug logs, pretty format

### Learning
- **Complete system**: See how everything connects
- **Experimentation**: Safe to break things
- **Configuration**: Learn without GCP complexity

## Migration Path

### From GCP to Local
1. Clone configuration files
2. Update environment variables
3. Start services locally
4. Run same tests as GCP

### From Local to GCP
1. Copy configuration structure
2. Update resource limits
3. Split into two instances
4. Update networking

## Success Criteria

### Must Have
- ✅ All 9 services start successfully
- ✅ WS Server connects to Redpanda
- ✅ Publisher generates events
- ✅ Messages flow end-to-end
- ✅ Grafana shows live data
- ✅ WebSocket clients can connect
- ✅ All 8 event types work

### Should Have
- ⚠️ Resource usage < 8GB RAM
- ⚠️ Health checks pass
- ⚠️ Logs visible in Grafana
- ⚠️ Metrics in Prometheus
- ⚠️ Setup takes < 5 minutes

### Nice to Have
- 🎯 One-command setup script
- 🎯 Auto-restart on failure
- 🎯 Volume persistence
- 🎯 Docker Compose profiles (minimal/full)

## Timeline Estimate

### Minimum Viable (1-2 hours)
- Create docker-compose.yml
- Copy configuration files
- Update environment variables
- Test basic startup

### Full Implementation (2-4 hours)
- Complete all phases
- Write documentation
- Create setup scripts
- Comprehensive testing

### Polish (1 hour)
- Troubleshooting guide
- Performance tuning
- Examples and demos

**Total: 4-7 hours** for complete local deployment setup

## Next Steps

1. **Create directory structure**
2. **Copy and adapt docker-compose files**
3. **Update configurations for local networking**
4. **Test service startup**
5. **Verify message flow**
6. **Document setup process**
7. **Create helper scripts**

This plan ensures the local setup mirrors GCP as closely as possible while adapting for local development constraints.
