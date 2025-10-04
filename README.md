# Odin WebSocket Server

Production-grade WebSocket server with NATS pub/sub, Prometheus metrics, Grafana visualization, and Loki log aggregation.

![Go 1.25.1](https://img.shields.io/badge/Go-1.25.1-00ADD8?logo=go)
![Node.js 22](https://img.shields.io/badge/Node.js-22-339933?logo=node.js)
![NATS 2.12](https://img.shields.io/badge/NATS-2.12-27AAE1)
![Prometheus 3.6](https://img.shields.io/badge/Prometheus-3.6-E6522C?logo=prometheus)
![Grafana 12.2](https://img.shields.io/badge/Grafana-12.2-F46800?logo=grafana)
![Loki 3.3](https://img.shields.io/badge/Loki-3.3-F46800?logo=grafana)

## 📁 Project Structure

```
├── /src/                 # Go WebSocket server (production)
├── /publisher/           # Node.js NATS publisher service
├── /node-server/         # Node.js WebSocket server (alternative implementation)
├── /scripts/             # Testing and utility scripts
├── /docs/                # Complete documentation
│   ├── /architecture/    # System design documents
│   ├── /monitoring/      # Monitoring guides
│   └── /development/     # Development guides
├── /grafana/             # Grafana dashboard provisioning
├── docker-compose.yml    # Docker orchestration
├── Taskfile.yml          # Task automation
└── prometheus.yml        # Prometheus configuration
```

## 🚀 Quick Start

### Prerequisites

- **Docker & Docker Compose** - [Install Docker](https://docs.docker.com/get-docker/)
- **Task** - [Install Task](https://taskfile.dev/installation/)
- **Go 1.25.1+** (for local development) - [Install Go](https://go.dev/doc/install)
- **Node.js 22+** (for publisher/scripts) - [Install Node.js](https://nodejs.org/)

### Start Everything

```bash
# Install dependencies
task install

# Start all services (NATS, Go server, Publisher, Prometheus, Grafana, Loki, Promtail)
task docker:up

# Run stress test
task test:medium

# Open monitoring dashboards
task monitor:grafana     # Metrics dashboard
task monitor:logs        # Logs dashboard

# Control publisher
task publisher:start RATE=10
```

That's it! Services will be available at:

- **WebSocket**: ws://localhost:3004/ws
- **Health**: http://localhost:3004/health
- **Grafana**: http://localhost:3010 (admin/admin)
- **Prometheus**: http://localhost:9091
- **Loki**: http://localhost:3101

## 📋 Available Commands

List all available tasks:
```bash
task --list
```

### Common Commands

```bash
# Development
task dev:go              # Run Go server locally
task dev:publisher       # Run publisher locally
task dev:nats            # Start only NATS for local dev

# Testing (with variable overrides)
task test:light                              # 100 connections, 30s
task test:medium                             # 500 connections, 60s
task test:heavy                              # 2000 connections, 120s
task test:custom CONNECTIONS=1000 DURATION=90 SERVER=go2  # Custom load

# Monitoring - Metrics
task monitor:health      # Check all health endpoints
task monitor:grafana     # Open Grafana (localhost:3010)
task monitor:prometheus  # Open Prometheus (localhost:9091)
task monitor:metrics     # View raw Prometheus metrics
task monitor:targets     # Check Prometheus scrape targets

# Monitoring - Logs
task monitor:logs        # Open Grafana logs dashboard
task monitor:loki CONTAINER=odin-ws-go QUERY=''  # Query Loki directly

# Publisher Control
task publisher:start RATE=10          # Start publishing (msgs/sec)
task publisher:stop                   # Stop publishing
task publisher:configure RATE=20      # Change publish rate
task monitor:publisher:stats          # View publisher statistics

# Docker
task docker:up           # Start all services
task docker:down         # Stop all services
task docker:logs         # View all logs
task docker:clean        # Remove all containers & volumes

# Building
task build:docker        # Build Docker images
task build:go            # Build Go binary locally
task build:publisher     # Compile TypeScript
```

See [Taskfile Guide](./docs/development/TASKFILE_GUIDE.md) for complete command reference.

## 🔗 Service URLs

| Service | URL | Description | Credentials |
|---------|-----|-------------|-------------|
| Go WebSocket | ws://localhost:3004/ws | Production WebSocket server | - |
| Go Health | http://localhost:3004/health | Health check endpoint | - |
| Go Metrics | http://localhost:3004/metrics | Prometheus metrics | - |
| Publisher API | http://localhost:3003/control | Publisher control API | - |
| Publisher Stats | http://localhost:3003/stats | Publisher statistics | - |
| Grafana | http://localhost:3010 | Monitoring dashboards | admin/admin |
| Prometheus | http://localhost:9091 | Metrics database | - |
| Loki | http://localhost:3101 | Log aggregation | - |
| NATS | nats://localhost:4222 | Message broker | - |

## 📚 Documentation

- **[Architecture](./docs/architecture/)** - System design, NATS flow, connection management
- **[Monitoring](./docs/monitoring/)** - Prometheus + Grafana setup and usage
- **[Development](./docs/development/)** - Local development, Taskfile guide, debugging
- **[Taskfile Guide](./docs/development/TASKFILE_GUIDE.md)** - Complete task reference
- **[Local Development](./docs/development/LOCAL_DEVELOPMENT.md)** - Running without Docker

### Key Documentation

- [NATS Architecture & Flow](./docs/architecture/ARCHITECTURE_NATS_FLOW.md)
- [Replay Mechanism Deep Dive](./docs/architecture/REPLAY_MECHANISM_DEEP_DIVE.md)
- [Monitoring Setup Guide](./docs/monitoring/MONITORING_SETUP.md)
- [Taskfile Complete Guide](./docs/development/TASKFILE_GUIDE.md)

## 🧪 Testing

### Quick Stress Tests

Simple load tests for quick validation:

```bash
# Light load (100 connections, 30 seconds)
task test:light

# Medium load (500 connections, 60 seconds)
task test:medium

# Heavy load (2000 connections, 120 seconds)
task test:heavy

# Custom load with variable overrides
task test:custom CONNECTIONS=1000 DURATION=90 SERVER=go2

# Override individual test parameters
CONNECTIONS=250 task test:light        # 250 connections, 30s
DURATION=120 task test:medium          # 500 connections, 120s
SERVER=go2 task test:heavy             # Use go2 server
```

### Realistic Trading Simulation ⭐

Simulates real-world crypto trading platform behavior with auto-balancing:

```bash
# Short test (5 minutes, 300 connections)
task test:realistic:short

# Medium test (30 minutes, 1000 connections)
task test:realistic:medium

# Long test (2 hours, 2000 connections)
task test:realistic:long

# Custom realistic test
task test:realistic TARGET_CONNECTIONS=1500 DURATION=3600
```

**Features:**
- ✅ **Auto-balancing**: Monitors server health and adjusts load automatically
- ✅ **Realistic patterns**: Simulates 5 trader types (quick traders, day traders, bots)
- ✅ **Gradual ramp-up**: Connections increase gradually, not instant
- ✅ **Variable sessions**: Users stay connected for realistic durations (30s to 48h)
- ✅ **Peak hours**: Automatically increases load during market peak hours
- ✅ **Reconnection logic**: 70% of disconnected users reconnect
- ✅ **Health monitoring**: Scales down if server is under stress

**Trader Types Distribution:**
- 20% Quick Traders (30s - 5min sessions)
- 35% Active Traders (5min - 30min sessions)
- 30% Day Traders (30min - 4h sessions)
- 10% Long Holders (4h - 8h sessions)
- 5% Bots/Algorithms (24h - 48h sessions)

**Monitor results in real-time:**
- **Metrics Dashboard**: http://localhost:3010 (Grafana - admin/admin)
- **Logs Dashboard**: `task monitor:logs` or http://localhost:3010/d/websocket-logs-v2

## 🎯 Architecture

```
┌─────────────┐       ┌──────────┐       ┌────────────────┐
│   Clients   │──WS───│ Go Server│◄──────│  Prometheus    │
│  (Browser)  │       │  :3004   │       │    :9091       │
└─────────────┘       └──────────┘       └────────────────┘
                           │                      │
                        ┌──▼──┐                   │
                        │NATS │                   │
                        │4222 │                   │
                        └──▲──┘           ┌───────▼────────┐
                           │              │    Grafana     │
                     ┌─────┴────┐         │     :3010      │
                     │Publisher │         │  (Dashboards)  │
                     │  :3003   │         └────────┬───────┘
                     └──────────┘                  │
                           │                       │
                  ┌────────┴────────┐       ┌──────▼──────┐
                  │    Promtail     │──────►│    Loki     │
                  │ (Log Collector) │       │   :3101     │
                  └─────────────────┘       └─────────────┘
```

**Key Components:**
- **Go WebSocket Server** - Production WebSocket server with connection management, replay mechanism, rate limiting
- **NATS** - Message broker for pub/sub communication
- **Publisher** - Simulates market data publishing for testing
- **Prometheus** - Metrics collection and storage
- **Grafana** - Real-time visualization and dashboards (metrics + logs)
- **Loki** - Log aggregation and storage
- **Promtail** - Log collection from Docker containers

## 🛠️ Development

### Local Development (without Docker)

```bash
# Terminal 1: Start NATS only
task dev:nats

# Terminal 2: Run Go server locally
task dev:go

# Terminal 3: Run publisher locally
task dev:publisher

# Terminal 4: Run tests
task test:light
```

See [Local Development Guide](./docs/development/LOCAL_DEVELOPMENT.md) for details.

### Build Docker Images

```bash
# Build all images
task build:docker

# Build specific service
task build:docker:go
task build:docker:publisher
```

### Hot Reload Development

**Go (with Air):**
```bash
cd src
air  # Requires air: go install github.com/air-verse/air@latest
```

**Publisher (with nodemon):**
```bash
cd publisher
nodemon --exec tsx publisher.ts
```

## 📊 Monitoring

Access Grafana at http://localhost:3010 (admin/admin) for complete observability.

### Metrics Dashboard (Prometheus)

Real-time metrics visualization:

- **Active WebSocket connections** - Real-time connection count
- **Message throughput** - Messages/sec sent and received
- **CPU and memory usage** - Resource utilization
- **Error rates** - Connection failures, slow clients
- **NATS connection status** - Message broker health

Dashboard panels:
1. Active Connections (Gauge)
2. Connections Over Time (Graph)
3. Message Rate (Graph)
4. Bandwidth (Graph)
5. CPU Usage (Graph)
6. Memory Usage (Graph)
7. Goroutines (Graph)
8. Reliability Metrics (Graph)
9. NATS Status (Gauge)

### Logs Dashboard (Loki)

Real-time log streaming with auto-refresh (5s):

```bash
# Open logs dashboard in browser
task monitor:logs

# Query logs directly from CLI
task monitor:loki CONTAINER=odin-ws-go
task monitor:loki CONTAINER=odin-publisher
task monitor:loki CONTAINER=odin-nats
```

**Available Log Panels:**
1. **Go WebSocket Server Logs** - All server logs
2. **Publisher Logs** - NATS publisher activity
3. **NATS Logs** - Message broker logs
4. **Message Broadcasts** - Filtered WebSocket message logs
5. **Price Updates** - Filtered publisher price updates

**Log Filtering Examples:**
- View only errors: Add `|~ "(?i)error"` to query
- View broadcasts: `{container_name="odin-ws-go"} |~ "(?i)(broadcast|message)"`
- View price updates: `{container_name="odin-publisher"} |~ "(?i)(publish|token|price)"`

See [Monitoring Setup Guide](./docs/monitoring/MONITORING_SETUP.md) for configuration details.

## 🔧 Configuration

### Software Versions

- **Go**: 1.25.1
- **Node.js**: 22
- **NATS**: 2.12
- **Prometheus**: 3.6.0
- **Grafana**: 12.2.0
- **Alpine Linux**: 3.20

### Environment Variables

**Go Server** (via command-line flags):
```bash
-addr=:3002                      # Server address
-nats=nats://localhost:4222      # NATS URL
```

**Publisher** (via `.env`):
```env
NATS_URL=nats://localhost:4222
PORT=3003
NODE_ENV=production
TOKENS=BTC,ETH,ODIN,SOL,DOGE
```

### Resource Limits

Configured in `docker-compose.yml`:

| Service | CPU Limit | Memory Limit |
|---------|-----------|--------------|
| Go Server | 2 cores | 512 MB |
| Publisher | 0.5 cores | 128 MB |
| NATS | 1 core | 256 MB |
| Prometheus | 1 core | 512 MB |
| Grafana | 0.5 cores | 256 MB |

## 🚦 Health Checks

Check service health:

```bash
# All services
task monitor:health

# Go server only
curl http://localhost:3004/health | jq '.'

# Publisher only
curl http://localhost:3003/health | jq '.'
```

Health endpoint response:
```json
{
  "status": "healthy",
  "healthy": true,
  "checks": {
    "nats": {"status": "connected", "healthy": true},
    "capacity": {"current": 0, "max": 2184, "percentage": 0, "healthy": true},
    "memory": {"used_mb": 12.4, "limit_mb": 512, "percentage": 2.4, "healthy": true},
    "cpu": {"percentage": 1.5, "healthy": true}
  },
  "warnings": [],
  "errors": [],
  "uptime": 123.45
}
```

## 🤝 Contributing

1. **Read Documentation** - Start with [docs/README.md](./docs/README.md)
2. **Follow Conventions** - Use `task format` before committing
3. **Test Changes** - Run stress tests to verify functionality
4. **Update Docs** - Keep documentation in sync with code changes

See [Local Development Guide](./docs/development/LOCAL_DEVELOPMENT.md) for setup.

## 📝 License

ISC

## 🔗 Additional Resources

- **Taskfile Documentation**: https://taskfile.dev/
- **Go Documentation**: https://go.dev/doc/
- **NATS Documentation**: https://docs.nats.io/
- **Prometheus Documentation**: https://prometheus.io/docs/
- **Grafana Documentation**: https://grafana.com/docs/
- **Docker Documentation**: https://docs.docker.com/

---

**Quick Commands:**

```bash
# Setup & Start
task setup              # First-time setup
task docker:up          # Start all services

# Testing & Load
task test:medium        # Run stress test (500 connections, 60s)
CONNECTIONS=1000 task test:custom  # Custom load test

# Monitoring
task monitor:grafana    # Open metrics dashboard
task monitor:logs       # Open logs dashboard
task monitor:health     # Check all health endpoints

# Publisher Control
task publisher:start RATE=10   # Start publishing (10 msgs/sec)
task publisher:stop            # Stop publishing

# Cleanup
task docker:down        # Stop services
task docker:clean       # Remove all containers & volumes
```

For help: `task --list`
