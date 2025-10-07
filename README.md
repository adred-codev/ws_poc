# Odin WebSocket Server

Production-grade WebSocket server with NATS pub/sub, Prometheus metrics, Grafana visualization, and Loki log aggregation.

![Go 1.25.1](https://img.shields.io/badge/Go-1.25.1-00ADD8?logo=go)
![Node.js 22](https://img.shields.io/badge/Node.js-22-339933?logo=node.js)
![NATS 2.12](https://img.shields.io/badge/NATS-2.12-27AAE1)
![Prometheus 3.6](https://img.shields.io/badge/Prometheus-3.6-E6522C?logo=prometheus)
![Grafana 12.2](https://img.shields.io/badge/Grafana-12.2-F46800?logo=grafana)
![Loki 3.3](https://img.shields.io/badge/Loki-3.3-F46800?logo=grafana)

## 🎯 Overview

A high-performance WebSocket server designed for real-time data streaming with enterprise-grade reliability and observability. Built for production use with comprehensive monitoring, automatic failover, and intelligent connection management.

### Key Features

- ✅ **High Performance** - Handles 2000+ concurrent connections with sub-10ms latency
- ✅ **Reliability** - Automatic reconnection, message replay, connection recovery
- ✅ **Observability** - Prometheus metrics + Grafana dashboards + Loki logs
- ✅ **Production Ready** - Docker Compose orchestration, health checks, resource limits
- ✅ **Developer Friendly** - Comprehensive docs, automated tasks, hot reload support
- ✅ **Cloud Native** - GCP deployment automation, systemd integration, auto-scaling ready

### Architecture Highlights

- **Token Bucket Rate Limiting** - Fair resource allocation per connection
- **Tiered Buffer Pools** - Memory-efficient message handling (4KB/16KB/64KB)
- **Worker Pool Pattern** - Bounded concurrency with graceful degradation
- **NATS Integration** - Decoupled message publishing with reliable delivery
- **Cgroup-Aware Limits** - Automatic capacity planning based on container resources
- **Connection Recovery** - Sequence-based message replay on reconnection

## 🏗️ Architecture

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

## 📁 Project Structure

```
├── /src/                 # Go WebSocket server (production)
├── /publisher/           # Node.js NATS publisher service
├── /node-server/         # Node.js WebSocket server (alternative implementation)
├── /scripts/             # Testing and utility scripts
├── /docs/                # Complete documentation
│   ├── /architecture/    # System design documents
│   ├── /deployment/      # Deployment guides
│   ├── /monitoring/      # Monitoring guides
│   └── /development/     # Development guides
├── /grafana/             # Grafana dashboard provisioning
├── /taskfiles/           # Modular task definitions
├── docker-compose.yml    # Docker orchestration
├── docker-compose.prod.yml # Production overrides
├── Taskfile.yml          # Main task orchestrator
└── prometheus.yml        # Prometheus configuration
```

## 🚀 Quick Start

### Prerequisites

- **Docker & Docker Compose** - [Install Docker](https://docs.docker.com/get-docker/)
- **Task** - [Install Task](https://taskfile.dev/installation/)
- **Go 1.25.1+** (for local development) - [Install Go](https://go.dev/doc/install)
- **Node.js 22+** (for publisher/scripts) - [Install Node.js](https://nodejs.org/)

### Get Started

```bash
# Complete setup (installs dependencies, builds images, starts services)
task setup

# Or manually:
task utils:install       # Install dependencies
task build:docker        # Build Docker images
task docker:up           # Start all services

# Run a test
task test:medium

# Open monitoring
task monitor:grafana     # Metrics dashboard (http://localhost:3010)
task monitor:logs        # Logs dashboard
```

**For detailed usage instructions, see [Local Development Guide](./docs/development/LOCAL_DEVELOPMENT.md).**

## 📚 Documentation

Complete documentation organized by topic:

### Getting Started
- **[Local Development Guide](./docs/development/LOCAL_DEVELOPMENT.md)** - Complete usage guide for local development with Docker
- **[Taskfile Guide](./docs/development/TASKFILE_GUIDE.md)** - Reference for all available task commands
- **[Taskfile Modules](./taskfiles/README.md)** - Modular task structure documentation

### Architecture & Design
- **[NATS Architecture & Flow](./docs/architecture/ARCHITECTURE_NATS_FLOW.md)** - System architecture and NATS integration
- **[Replay Mechanism Deep Dive](./docs/architecture/REPLAY_MECHANISM_DEEP_DIVE.md)** - Message replay and gap recovery
- **[Connection Limit Explained](./docs/architecture/CONNECTION_LIMIT_EXPLAINED.md)** - Connection capacity planning
- **[Connection Cleanup Explained](./docs/architecture/CONNECTION_CLEANUP_EXPLAINED.md)** - Connection lifecycle management
- **[Full Reconnect Explained](./docs/architecture/FULL_RECONNECT_EXPLAINED.md)** - Client reconnection flows

### Production Deployment
- **[GCP Deployment Guide](./docs/deployment/GCP_DEPLOYMENT.md)** - Automated GCP deployment with task commands
- **[Monitoring Setup Guide](./docs/monitoring/MONITORING_SETUP.md)** - Prometheus + Grafana + Loki configuration

### Quick Links
```bash
# View all available commands
task --list

# Local development guide
open docs/development/LOCAL_DEVELOPMENT.md

# GCP deployment guide
open docs/deployment/GCP_DEPLOYMENT.md

# Architecture overview
open docs/README.md
```

## 🧪 Technology Stack

### Backend
- **Go 1.25.1** - WebSocket server with high concurrency
- **NATS 2.12** - Message broker for pub/sub
- **Node.js 22** - Publisher service and test scripts

### Observability
- **Prometheus 3.6** - Metrics collection and storage
- **Grafana 12.2** - Metrics and logs visualization
- **Loki 3.3** - Log aggregation
- **Promtail** - Log collection from Docker containers

### Infrastructure
- **Docker** - Container orchestration
- **Docker Compose** - Multi-container management
- **Alpine Linux 3.20** - Minimal production images
- **Systemd** - Production service management (GCP)

## 🔧 Configuration

### Resource Limits

Configured in `docker-compose.yml`:

| Service | CPU Limit | Memory Limit |
|---------|-----------|--------------|
| Go Server | 2 cores | 512 MB |
| Publisher | 0.5 cores | 128 MB |
| NATS | 1 core | 256 MB |
| Prometheus | 1 core | 512 MB |
| Grafana | 0.5 cores | 256 MB |

### Environment Variables

**Go Server** (command-line flags):
- `-addr` - Server address (default: `:3002`)
- `-nats` - NATS URL (default: `nats://localhost:4222`)

**Publisher** (.env file):
- `NATS_URL` - NATS connection URL
- `PORT` - HTTP API port (default: `3003`)
- `NODE_ENV` - Environment (development/production)
- `TOKENS` - Comma-separated token list (e.g., `BTC,ETH,ODIN,SOL,DOGE`)

See [Local Development Guide](./docs/development/LOCAL_DEVELOPMENT.md#environment-variables) for details.

## 🤝 Contributing

1. **Read Documentation** - Start with [docs/README.md](./docs/README.md)
2. **Follow Conventions** - Use `task format` before committing
3. **Test Changes** - Run stress tests to verify functionality
4. **Update Docs** - Keep documentation in sync with code changes

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

**Need Help?**

- **Getting Started**: See [Local Development Guide](./docs/development/LOCAL_DEVELOPMENT.md)
- **All Commands**: Run `task --list` or see [Taskfile Guide](./docs/development/TASKFILE_GUIDE.md)
- **GCP Deployment**: See [GCP Deployment Guide](./docs/deployment/GCP_DEPLOYMENT.md)
- **Architecture**: See [Architecture Documentation](./docs/architecture/)
