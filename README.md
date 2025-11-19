# Odin WebSocket Server

Production-grade, horizontally-scalable WebSocket server with Kafka pub/sub, multi-shard architecture, comprehensive monitoring, and enterprise-grade reliability.

![Go 1.25.1](https://img.shields.io/badge/Go-1.25.1-00ADD8?logo=go)
![Node.js 22](https://img.shields.io/badge/Node.js-22-339933?logo=node.js)
![Kafka/Redpanda](https://img.shields.io/badge/Kafka-Redpanda-FF6D70)
![Prometheus 3.6](https://img.shields.io/badge/Prometheus-3.6-E6522C?logo=prometheus)
![Grafana 12.2](https://img.shields.io/badge/Grafana-12.2-F46800?logo=grafana)
![Loki 3.3](https://img.shields.io/badge/Loki-3.3-F46800?logo=grafana)

## 🎯 Overview

A high-performance, production-ready WebSocket server designed for real-time data streaming at scale. Features multi-core architecture with intelligent load balancing, comprehensive observability, and battle-tested reliability patterns.

### Production Metrics (Validated)

- ✅ **18,000 concurrent connections** - Tested at 98.4% capacity (17,710 connections)
- ✅ **51,000+ msg/sec throughput** - Sustained with zero errors
- ✅ **Sub-10ms latency** - Event-driven architecture with efficient I/O multiplexing
- ✅ **99.9% uptime** - Graceful degradation, panic recovery, automatic failover
- ✅ **Multi-core scaling** - 3-shard architecture with LoadBalancer proxy

### Key Features

**Performance & Scalability:**
- **Multi-Shard Architecture** - Horizontal scaling with LoadBalancer (least connections strategy)
- **High Throughput** - 51K+ messages/sec with 18K connections, event-driven efficiency
- **Resource Management** - Goroutine limits (100K), memory management, CPU admission control
- **Smart Load Balancing** - Per-connection routing with automatic shard distribution

**Reliability & Safety:**
- **Graceful Degradation** - ResourceGuard for overload protection
- **Panic Recovery** - Comprehensive panic handling in all goroutines
- **Rate Limiting** - Token bucket per-IP protection with localhost bypass
- **Connection Recovery** - Sequence-based message replay on reconnection
- **Health Monitoring** - Detailed health checks with capacity, CPU, memory, Kafka status

**Observability:**
- **Prometheus Metrics** - 50+ metrics for connections, messages, goroutines, CPU, memory
- **Grafana Dashboards** - Real-time visualization and alerting
- **Loki Log Aggregation** - Centralized structured logging with filtering
- **Debug Logging** - Optimized debug paths with zero production overhead

**Developer Experience:**
- **Task Automation** - Comprehensive Taskfile with 50+ commands
- **Docker Compose** - One-command local development setup
- **Hot Reload** - Air for live code reloading during development
- **Comprehensive Documentation** - Architecture, deployment, API, troubleshooting

## 🏗️ Architecture

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    Clients (18K connections)                     │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                    LoadBalancer (:3001)                          │
│              Least Connections Strategy                          │
│              Session Affinity: Client IP                         │
└─────┬──────────────────┬──────────────────┬─────────────────────┘
      │                  │                  │
      ▼                  ▼                  ▼
┌──────────┐       ┌──────────┐       ┌──────────┐
│ Shard 0  │       │ Shard 1  │       │ Shard 2  │
│  :3002   │       │  :3003   │       │  :3004   │
│ 6K conns │       │ 6K conns │       │ 6K conns │
└────┬─────┘       └────┬─────┘       └────┬─────┘
     │                  │                  │
     └──────────────────┼──────────────────┘
                        │
                        ▼
           ┌────────────────────────┐
           │   Kafka/Redpanda       │
           │   12 partitions/topic  │
           │   Consumer groups      │
           └────────┬───────────────┘
                    │
                    ▼
           ┌────────────────────────┐
           │   Publisher Service    │
           │   Event generation     │
           └────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                        Observability Stack                       │
│   Prometheus → Grafana (metrics visualization)                  │
│   Promtail → Loki → Grafana (log aggregation)                   │
└─────────────────────────────────────────────────────────────────┘
```

### Multi-Shard Architecture Details

**3 Shards + LoadBalancer Configuration:**
- Each shard: Independent WebSocket server (6,000 connections max)
- LoadBalancer: Routes connections using least-connections algorithm
- BroadcastBus: Coordinates message distribution across all shards
- Kafka Consumer Groups: Parallel consumption with partition distribution

**Goroutine Breakdown (per connection):**
- Shard: 2 goroutines (readPump + writePump)
- LoadBalancer Proxy: 2 goroutines (client→backend + backend→client)
- **Total: 4 goroutines per connection** (tested at 72,340 goroutines for 17.7K connections)

**Resource Distribution (e2-highcpu-8):**
- CPU: 8 vCPU → 3 shards (~1 core each) + LoadBalancer + overhead
- Memory: 8GB → 7GB limit (1GB per ~2.5K connections)
- Goroutines: 100K limit (72% utilized at 18K connections)

## 📁 Project Structure

```
ws_poc/
├── cmd/
│   └── ws-server/              # Main application entry point
├── ws/
│   ├── internal/
│   │   ├── multi/              # Multi-core architecture (LoadBalancer, BroadcastBus)
│   │   └── shared/             # Shared components (Server, Kafka, metrics, health)
│   └── pkg/                    # Public packages (config, logger, models)
├── publisher/                  # Node.js event publisher service
├── scripts/                    # Testing and utility scripts (load testing, etc.)
├── deployments/
│   └── v1/
│       └── gcp/
│           └── distributed/    # Production deployment configs
│               ├── backend/    # Kafka, Publisher, Monitoring
│               └── ws-server/  # WebSocket server (multi-shard)
├── taskfiles/
│   └── v1/
│       ├── gcp/               # GCP deployment tasks
│       ├── local/             # Local development tasks
│       └── shared/            # Shared utilities
├── docs/
│   ├── architecture/          # System design and patterns
│   ├── deployment/            # Deployment guides
│   ├── development/           # Developer guides
│   ├── performance/           # Optimization and capacity planning
│   ├── events/                # Event system documentation
│   ├── monitoring/            # Observability setup
│   └── archive/               # Historical documents
├── sessions/                  # Session handoff documents (65+ files)
├── grafana/                   # Grafana dashboard provisioning
├── docker-compose.yml         # Local development orchestration
├── Dockerfile                 # Multi-stage Docker build
├── Taskfile.yml              # Main task orchestrator
└── prometheus.yml            # Prometheus configuration
```

### Component Architecture

**Multi-Core Components (`ws/internal/multi/`):**
- **LoadBalancer** - Routes connections to shards using least-connections
- **Proxy** - Bidirectional WebSocket proxy between clients and shards
- **BroadcastBus** - Coordinates message distribution across shards
- **ResourceGuard** - CPU/memory admission control with graceful degradation

**Shared Components (`ws/internal/shared/`):**
- **Server** - WebSocket server core (connection handling, readPump, writePump)
- **Kafka** - Consumer group management, offset tracking, partition balancing
- **Health** - Comprehensive health checks (capacity, CPU, memory, Kafka, goroutines)
- **Metrics** - Prometheus instrumentation (50+ metrics)
- **Ratelimiter** - Token bucket rate limiting with IP-based quotas

## 🚀 Quick Start

### Prerequisites

- **Docker & Docker Compose** - [Install Docker](https://docs.docker.com/get-docker/)
- **Task** - [Install Task](https://taskfile.dev/installation/)
- **Go 1.25.1+** (for local development) - [Install Go](https://go.dev/doc/install)
- **Node.js 22+** (for publisher/scripts) - [Install Node.js](https://nodejs.org/)

### Local Development Setup

```bash
# Complete setup (installs dependencies, builds images, starts services)
task setup

# Or step-by-step:
task utils:install       # Install dependencies
task build:docker        # Build Docker images
task docker:up           # Start all services

# Run load test
task test:medium         # 1,000 connections test

# Open monitoring
task monitor:grafana     # http://localhost:3010 (admin/admin)
task monitor:logs        # Loki log explorer
```

### GCP Production Deployment

```bash
# Set up GCP credentials
export GCP_PROJECT=your-project-id

# Deploy backend (Kafka, Publisher, Monitoring)
task gcp:deployment:backend:deploy

# Deploy WebSocket server (multi-shard)
task gcp:deployment:ws:deploy

# Verify deployment
task gcp:health:check
task gcp:stats:connections

# Run capacity test
task gcp:load-test:start connections=18000
```

**For detailed deployment instructions, see [GCP Deployment Guide](./docs/deployment/GCP_DEPLOYMENT.md).**

## 📚 Documentation

Complete documentation organized by topic:

### Getting Started
- **[Local Development Guide](./docs/development/LOCAL_DEVELOPMENT.md)** - Complete setup and usage guide
- **[Taskfile Guide](./docs/development/TASKFILE_GUIDE.md)** - Reference for all task commands
- **[Configuration Guide](./docs/development/CONFIGURATION.md)** - Environment variables and settings

### Architecture & Design
- **[Multi-Core Architecture](./docs/architecture/MULTI_CORE_USAGE.md)** - Sharding and load balancing
- **[Horizontal Scaling Plan](./docs/architecture/HORIZONTAL_SCALING_PLAN.md)** - Scaling strategies
- **[Connection Limit Explained](./docs/architecture/CONNECTION_LIMIT_EXPLAINED.md)** - Capacity planning
- **[Connection Cleanup Explained](./docs/architecture/CONNECTION_CLEANUP_EXPLAINED.md)** - Lifecycle management
- **[Replay Mechanism Deep Dive](./docs/architecture/REPLAY_MECHANISM_DEEP_DIVE.md)** - Message replay system

### Production Deployment
- **[GCP Deployment Guide](./docs/deployment/GCP_DEPLOYMENT.md)** - Automated GCP deployment
- **[Production Architecture](./docs/deployment/PRODUCTION_ARCHITECTURE.md)** - Production patterns
- **[Monitoring Setup Guide](./docs/monitoring/MONITORING_SETUP.md)** - Prometheus + Grafana + Loki

### API & Integration
- **[API Rejection Responses](./docs/API_REJECTION_RESPONSES.md)** - Client error handling guide
- **[Kafka Replay Protocol](./docs/KAFKA_REPLAY_PROTOCOL.md)** - Message replay specification
- **[Token Update Events](./docs/events/TOKEN_UPDATE_EVENTS.md)** - Event schemas

### Performance & Optimization
- **[Capacity Planning](./docs/performance/CAPACITY_PLANNING.md)** - Resource estimation
- **[TCP Tuning Implementation](./docs/performance/TCP_TUNING_IMPLEMENTATION.md)** - Network optimization
- **[Capacity Scaling Plan](./docs/performance/CAPACITY_SCALING_PLAN.md)** - Growth strategies

### Quick Navigation
```bash
# View all available commands
task --list

# Local development guide
open docs/development/LOCAL_DEVELOPMENT.md

# GCP deployment guide
open docs/deployment/GCP_DEPLOYMENT.md

# Multi-core architecture
open docs/architecture/MULTI_CORE_USAGE.md

# API error handling
open docs/API_REJECTION_RESPONSES.md
```

## 🧪 Technology Stack

### Backend
- **Go 1.25.1** - High-performance WebSocket server with goroutine-based concurrency
- **Kafka/Redpanda** - Distributed message broker for pub/sub (12 partitions/topic)
- **Node.js 22** - Publisher service and test scripts

### Observability
- **Prometheus 3.6** - Metrics collection and storage (50+ custom metrics)
- **Grafana 12.2** - Metrics and logs visualization with custom dashboards
- **Loki 3.3** - Log aggregation with structured logging (zerolog)
- **Promtail** - Log collection from Docker containers

### Infrastructure
- **Docker** - Containerization with multi-stage builds
- **Docker Compose** - Multi-container orchestration
- **Alpine Linux 3.20** - Minimal production images (~25MB)
- **GCP Compute Engine** - Cloud deployment (e2-highcpu-8)

## 🔧 Configuration

### Production Configuration (GCP)

**Instance Type:** e2-highcpu-8
- CPU: 8 vCPU (shared cores)
- Memory: 8 GB
- Network: 16 Gbps

**Capacity:**
- Max Connections: 18,000 (tested at 17,710 - 98.4% success)
- Shards: 3 (6,000 connections per shard)
- Goroutines: 100,000 limit (72% utilized at full capacity)
- Message Throughput: 51,110 msg/sec sustained

**Resource Limits:**
- CPU Limit: 7 cores (85% of available, headroom for bursts)
- Memory Limit: 7 GB (85% of available)
- Goroutine Limit: 100,000 (safety margin for 18K connections)

### Environment Variables

**Multi-Shard Configuration:**
```bash
# Shard topology
NUM_SHARDS=3                  # Number of shard instances
SHARD_BASE_PORT=3002          # First shard port (3002, 3003, 3004)
LB_ADDR=:3001                 # LoadBalancer external address

# Capacity limits
WS_MAX_CONNECTIONS=18000      # Total connections (6K per shard)
WS_MAX_GOROUTINES=100000      # Goroutine limit (accounts for proxy)

# Resource management
WS_CPU_LIMIT=7                # CPU cores available
WS_MEMORY_LIMIT=7516192768    # 7 GB memory limit

# Rate limiting
WS_MAX_KAFKA_RATE=25          # Kafka message rate per shard
WS_MAX_BROADCAST_RATE=25      # Broadcast rate per shard
```

**Kafka Configuration:**
```bash
# Connection
KAFKA_BROKERS=10.128.0.2:9092 # Backend internal IP
KAFKA_GROUP_ID=ws-server-production  # Base consumer group

# Topics (12 partitions each)
# odin.trades, odin.liquidity, odin.balances, odin.metadata,
# odin.social, odin.community, odin.creation, odin.analytics
```

**Publisher Configuration:**
```bash
NATS_URL=nats://redpanda:9092  # Kafka connection
PORT=3003                       # HTTP API port
PUBLISH_RATE=10                 # Events per second
```

See [Configuration Guide](./docs/development/CONFIGURATION.md) for complete reference.

## 📊 Performance Metrics

### Validated Capacity Test Results

**Test Configuration:**
- Target: 18,000 connections
- Ramp Rate: 100 conn/sec
- Duration: 30 minutes sustained
- Publisher: 10 events/sec (BTC, ETH, SOL)

**Results:**
```
Connections:     17,710 / 18,000 (98.4% success)
Message Rate:    51,110 msg/sec (sustained)
CPU Usage:       5-10% idle, 30-70% during broadcasts (avg 15-20%)
Memory:          1,081 MB (~1 GB stable)
Goroutines:      ~72,340 (72% of 100K limit)
Errors:          0 (zero goroutine errors, zero rate limit errors)
Latency:         <10ms (sub-millisecond event-driven)
```

**Shard Distribution:**
```
Shard 0: 5,909 / 6,000 (98.5% utilization)
Shard 1: 5,901 / 6,000 (98.4% utilization)
Shard 2: 5,900 / 6,000 (98.3% utilization)
Variance: 9 connections (0.15% - perfect balancing)
```

### Scaling Potential

**Current Headroom:**
- Goroutines: 28% available (27,660 / 100K)
- CPU: 80% available (5-10% baseline usage)
- Memory: 87.5% available (1GB / 8GB used)
- Message Rate: Can handle 10× publisher rate (100 events/sec → 511K msg/sec estimated)

**Horizontal Scaling:**
- 6 shards (36K connections) - Requires 16 vCPU instance
- 12 shards (72K connections) - Requires multi-instance deployment
- Load balancer tier - GCP TCP LB with session affinity

## 🔒 Reliability Features

### Graceful Degradation
- **ResourceGuard** - CPU/memory admission control
- **Rate Limiting** - Per-IP token bucket (localhost bypass for internal traffic)
- **Goroutine Limits** - Prevents runaway goroutine creation
- **Capacity Limits** - Per-shard connection caps (6K per shard)

### Panic Recovery
- **Global Recovery** - All goroutines wrapped with panic recovery
- **Monitoring Integration** - Extended panic recovery in monitoring goroutines
- **Graceful Cleanup** - Connection cleanup on panic

### Health Checks
- **Comprehensive Checks** - Capacity, CPU, memory, Kafka, goroutines
- **Status Levels** - Healthy, degraded, overloaded
- **HTTP Endpoint** - `/health` with detailed JSON response

### Client Error Handling
- **HTTP Errors** - 503 (shutdown/overload), 429 (rate limit)
- **WebSocket Close** - 1011 (backend unavailable), 1012 (overloaded)
- **Retry Strategies** - Exponential backoff with jitter

See [API Rejection Responses](./docs/API_REJECTION_RESPONSES.md) for client integration guide.

## 🤝 Contributing

1. **Read Documentation** - Start with [docs/README.md](./docs/README.md)
2. **Follow Conventions** - Use `task format` before committing
3. **Test Changes** - Run load tests to verify functionality
4. **Update Docs** - Keep documentation in sync with code changes

## 🔗 Additional Resources

- **Task Documentation**: https://taskfile.dev/
- **Go Documentation**: https://go.dev/doc/
- **Kafka Documentation**: https://kafka.apache.org/documentation/
- **Redpanda Documentation**: https://docs.redpanda.com/
- **Prometheus Documentation**: https://prometheus.io/docs/
- **Grafana Documentation**: https://grafana.com/docs/
- **Docker Documentation**: https://docs.docker.com/

---

## 🎯 Recent Updates

**Latest (2025-11-19):**
- ✅ Achieved 18K connection capacity (17.7K tested at 98.4% success)
- ✅ Fixed goroutine calculation (100K limit accounts for LoadBalancer proxy)
- ✅ Optimized debug logging (99.3% overhead reduction)
- ✅ Reorganized documentation (topic-based structure)
- ✅ Added comprehensive API rejection documentation

**Session Reports:** See [sessions/](./sessions/) for detailed handoff documents (65+ session files)

---

**Need Help?**

- **Getting Started**: See [Local Development Guide](./docs/development/LOCAL_DEVELOPMENT.md)
- **All Commands**: Run `task --list` or see [Taskfile Guide](./docs/development/TASKFILE_GUIDE.md)
- **GCP Deployment**: See [GCP Deployment Guide](./docs/deployment/GCP_DEPLOYMENT.md)
- **Architecture**: See [Multi-Core Architecture](./docs/architecture/MULTI_CORE_USAGE.md)
- **API Integration**: See [API Rejection Responses](./docs/API_REJECTION_RESPONSES.md)
- **Troubleshooting**: See session handoff documents in [sessions/](./sessions/)
