# Isolated Two-Instance Setup

This directory contains Docker Compose manifests for running the WebSocket server in an isolated two-instance architecture.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    GCP Project (same)                        │
│                                                              │
│  ┌────────────────────┐          ┌─────────────────────┐   │
│  │  odin-ws-go        │          │  odin-backend        │   │
│  │  (e2-small)        │          │  (e2-small)          │   │
│  │                    │          │                      │   │
│  │  • ws-go           │◄─────────┤  • NATS (JetStream) │   │
│  │                    │  internal│  • Publisher         │   │
│  │                    │   VPC    │  • Prometheus        │   │
│  │                    ├─────────►│  • Grafana           │   │
│  │                    │  metrics │  • Loki              │   │
│  │                    │  scrape  │  • Promtail          │   │
│  └────────────────────┘          └─────────────────────┘   │
│         ▲                                  ▲                │
│         │ ws://                            │ http://        │
│         │ :3004/ws                         │ :3010          │
└─────────┼──────────────────────────────────┼────────────────┘
          │                                  │
       Clients                          Monitoring UI
```

## Purpose

**Isolate ws-go for accurate metrics measurement:**
- ws-go runs on dedicated instance with full resource access
- Backend services run separately to avoid metric pollution
- Internal VPC communication (<1ms latency, no performance impact)
- Clean measurement of ws-go's actual capacity and resource usage

## Instances

### odin-ws-go (e2-small)
- **Services:** WebSocket server only
- **Resources:** 2.0 vCPU, 2 GB RAM
- **Config:** 10,000 max connections, 256 workers
- **External Port:** 3004 (WebSocket + metrics)
- **Cost:** $12.23/month

### odin-backend (e2-small)
- **Services:** NATS, Publisher, Prometheus, Grafana, Loki, Promtail
- **Resources:** 2.0 vCPU, 2 GB RAM
- **External Ports:** 3010 (Grafana), 3003 (Publisher)
- **Internal Port:** 4222 (NATS - exposed on 0.0.0.0 for ws-go)
- **Cost:** $12.23/month

**Total Cost:** $24.46/month

## Directory Structure

```
isolated/
├── README.md                    # This file
├── ws-go/
│   └── docker-compose.yml       # WebSocket server manifest
└── backend/
    ├── docker-compose.yml       # Backend services manifest
    └── prometheus.yml           # Prometheus config (scrapes remote ws-go)
```

## Template Variables

Both manifests use environment variable substitution:

**ws-go/docker-compose.yml:**
- `${BACKEND_INTERNAL_IP}` - Internal IP of backend instance (for NATS connection)

**backend/prometheus.yml:**
- `${WS_GO_INTERNAL_IP}` - Internal IP of ws-go instance (for metrics scraping)

## Deployment

Use the taskfile for automated deployment:

```bash
# Full setup (create instances + deploy services)
task -t taskfiles/isolated-setup.yml create:all
task -t taskfiles/isolated-setup.yml deploy:all

# Or step by step
task -t taskfiles/isolated-setup.yml setup          # Configure gcloud
task -t taskfiles/isolated-setup.yml firewall       # Create firewall rules
task -t taskfiles/isolated-setup.yml create:backend # Create backend instance
task -t taskfiles/isolated-setup.yml create:ws-go   # Create ws-go instance
task -t taskfiles/isolated-setup.yml deploy:backend # Deploy backend services
task -t taskfiles/isolated-setup.yml deploy:ws-go   # Deploy ws-go

# Health check
task -t taskfiles/isolated-setup.yml health:all

# Testing
task -t taskfiles/isolated-setup.yml test:light     # 1k connections
task -t taskfiles/isolated-setup.yml test:capacity  # 10k connections
```

## Manual Deployment

If deploying manually without taskfile:

### 1. Deploy Backend

```bash
# SSH to backend instance
gcloud compute ssh odin-backend --zone=us-central1-a

# Copy files
# (Copy docker-compose.yml, prometheus.yml, loki-config.yml, promtail-config.yml, grafana/, publisher/)

# Get ws-go internal IP
WS_GO_INTERNAL_IP=$(gcloud compute instances describe odin-ws-go \
  --zone=us-central1-a \
  --format='get(networkInterfaces[0].networkIP)')

# Substitute and deploy
export WS_GO_INTERNAL_IP
envsubst < prometheus.yml > prometheus.deployed.yml
mv prometheus.deployed.yml prometheus.yml
docker compose up -d
```

### 2. Deploy ws-go

```bash
# SSH to ws-go instance
gcloud compute ssh odin-ws-go --zone=us-central1-a

# Copy files
# (Copy docker-compose.yml, src/)

# Get backend internal IP
BACKEND_INTERNAL_IP=$(gcloud compute instances describe odin-backend \
  --zone=us-central1-a \
  --format='get(networkInterfaces[0].networkIP)')

# Substitute and deploy
export BACKEND_INTERNAL_IP
envsubst < docker-compose.yml | docker compose -f - up -d
```

## Network Communication

**Internal VPC (10.128.0.0/20):**
- ws-go → backend:4222 (NATS client connection)
- backend → ws-go:3002 (Prometheus scraping)
- Latency: <1ms (same zone)

**External Access:**
- Clients → ws-go:3004 (WebSocket connections)
- Browser → backend:3010 (Grafana UI)
- Tests → backend:3003 (Publisher API)

## Firewall Rules

- `allow-websocket-isolated` - External → ws-go:3004
- `allow-nats-internal` - ws-go → backend:4222 (internal only)
- `allow-prometheus-scrape-isolated` - backend → ws-go:3002 (internal only)
- `allow-grafana-isolated` - External → backend:3010
- `allow-publisher-isolated` - External → backend:3003

## Comparison with Shared Setup

| Aspect | Shared (Current) | Isolated (This) |
|--------|------------------|-----------------|
| Instances | 1 × e2-small | 2 × e2-small |
| ws-go CPU | ~1.5 shared | 1.8 dedicated |
| ws-go Memory | ~768MB shared | 1792MB dedicated |
| Max Connections | 2,000 | 10,000 |
| Metrics | Polluted | Clean |
| Cost/month | $12.23 | $24.46 |

## When to Use This Setup

**Use isolated setup when:**
- ✅ Need accurate ws-go performance metrics
- ✅ Testing capacity and scaling limits
- ✅ Preparing for production deployment
- ✅ Budget allows ~$25/month

**Use shared setup when:**
- ✅ Development and initial testing
- ✅ Budget-conscious ($12/month)
- ✅ <2,000 concurrent connections
- ✅ Don't need isolated metrics

## Migration from Shared

1. Create isolated instances (both stay running)
2. Test isolated setup
3. Switch client traffic to new ws-go
4. Delete old shared instance
5. Total downtime: ~0 (blue-green deployment)

## Cleanup

```bash
# Delete instances
task -t taskfiles/isolated-setup.yml delete:all

# Delete firewall rules
task -t taskfiles/isolated-setup.yml cleanup:firewall

# Or delete everything
task -t taskfiles/isolated-setup.yml cleanup:all
```

## See Also

- [../docs/INSTANCE_ISOLATION_PLAN.md](../docs/INSTANCE_ISOLATION_PLAN.md) - Detailed migration plan
- [../docs/CONFIG_CALCULATIONS.md](../docs/CONFIG_CALCULATIONS.md) - Configuration calculations
- [../docs/CAPACITY_PLANNING.md](../docs/CAPACITY_PLANNING.md) - Resource scaling formulas
