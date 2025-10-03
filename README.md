# Odin WebSocket Server

Production-grade WebSocket server with NATS pub/sub, Prometheus monitoring, and Grafana visualization.

![Go 1.25.1](https://img.shields.io/badge/Go-1.25.1-00ADD8?logo=go)
![Node.js 22](https://img.shields.io/badge/Node.js-22-339933?logo=node.js)
![NATS 2.12](https://img.shields.io/badge/NATS-2.12-27AAE1)
![Prometheus 3.6](https://img.shields.io/badge/Prometheus-3.6-E6522C?logo=prometheus)
![Grafana 12.2](https://img.shields.io/badge/Grafana-12.2-F46800?logo=grafana)

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

# Start all services (NATS, Go server, Publisher, Prometheus, Grafana)
task docker:up

# Run stress test
task test:medium

# Open Grafana dashboard
task monitor:grafana
```

That's it! Services will be available at:

- **WebSocket**: ws://localhost:3004/ws
- **Health**: http://localhost:3004/health
- **Grafana**: http://localhost:3010 (admin/admin)
- **Prometheus**: http://localhost:9091

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

# Testing
task test:light          # 100 connections, 30s
task test:medium         # 500 connections, 60s
task test:heavy          # 2000 connections, 120s

# Monitoring
task monitor:health      # Check all health endpoints
task monitor:grafana     # Open Grafana (localhost:3010)
task monitor:prometheus  # Open Prometheus (localhost:9091)

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

Run stress tests to generate load and visualize metrics:

```bash
# Light load (100 connections, 30 seconds)
task test:light

# Medium load (500 connections, 60 seconds)
task test:medium

# Heavy load (2000 connections, 120 seconds)
task test:heavy

# Custom load
task test:custom CONNECTIONS=1000 DURATION=90
```

Monitor results in Grafana: http://localhost:3010

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
                        └──▲──┘                   │
                           │              ┌───────▼────┐
                     ┌─────┴────┐         │  Grafana   │
                     │Publisher │         │   :3010    │
                     │  :3003   │         └────────────┘
                     └──────────┘
```

**Key Components:**
- **Go WebSocket Server** - Production WebSocket server with connection management, replay mechanism, rate limiting
- **NATS** - Message broker for pub/sub communication
- **Publisher** - Simulates market data publishing for testing
- **Prometheus** - Metrics collection and storage
- **Grafana** - Real-time visualization and dashboards

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

Access Grafana at http://localhost:3010 (admin/admin) to view:

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
task setup         # First-time setup
task docker:up     # Start services
task test:medium   # Run stress test
task monitor:grafana  # Open dashboard
task docker:down   # Stop services
```

For help: `task --list`
