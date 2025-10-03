# Documentation

Complete documentation for the Odin WebSocket Server project.

## 📚 Table of Contents

### Architecture
System design, data flow, and technical deep-dives

- [NATS Architecture & Flow](./architecture/ARCHITECTURE_NATS_FLOW.md) - System architecture and NATS integration
- [Connection Limit Explained](./architecture/CONNECTION_LIMIT_EXPLAINED.md) - How connection limits are calculated and enforced
- [Connection Cleanup Explained](./architecture/CONNECTION_CLEANUP_EXPLAINED.md) - Connection cleanup mechanisms
- [Full Reconnect Explained](./architecture/FULL_RECONNECT_EXPLAINED.md) - Client reconnection and recovery flows
- [Replay Mechanism Deep Dive](./architecture/REPLAY_MECHANISM_DEEP_DIVE.md) - Message replay and gap recovery system

### Monitoring
Prometheus + Grafana setup and usage

- [Monitoring Setup](./monitoring/MONITORING_SETUP.md) - Complete Prometheus + Grafana setup guide

### Development
Local development and workflow guides

- [Taskfile Guide](./development/TASKFILE_GUIDE.md) - Complete reference for all Taskfile commands
- [Local Development](./development/LOCAL_DEVELOPMENT.md) - Running and developing locally without Docker

## Quick Links

### Getting Started
1. [Installation & Setup](#) - See root README.md
2. [Taskfile Guide](./development/TASKFILE_GUIDE.md) - Learn the task commands
3. [Local Development](./development/LOCAL_DEVELOPMENT.md) - Set up local environment

### Running the Server
1. [Docker Setup](#) - See root README.md for `task docker:up`
2. [Monitoring Setup](./monitoring/MONITORING_SETUP.md) - Access Grafana dashboards
3. [Running Tests](#) - See root README.md for test commands

### Understanding the System
1. [NATS Architecture](./architecture/ARCHITECTURE_NATS_FLOW.md) - How messages flow through the system
2. [Replay Mechanism](./architecture/REPLAY_MECHANISM_DEEP_DIVE.md) - How message recovery works
3. [Connection Management](./architecture/CONNECTION_CLEANUP_EXPLAINED.md) - How connections are managed

## Documentation Structure

```
/docs/
  ├── README.md                    # This file
  ├── /architecture/               # System design documents
  │   ├── ARCHITECTURE_NATS_FLOW.md
  │   ├── CONNECTION_LIMIT_EXPLAINED.md
  │   ├── CONNECTION_CLEANUP_EXPLAINED.md
  │   ├── FULL_RECONNECT_EXPLAINED.md
  │   └── REPLAY_MECHANISM_DEEP_DIVE.md
  ├── /monitoring/                 # Monitoring guides
  │   └── MONITORING_SETUP.md
  └── /development/                # Development guides
      ├── TASKFILE_GUIDE.md
      └── LOCAL_DEVELOPMENT.md
```

## Contributing to Documentation

When adding new documentation:

1. **Architecture docs** → `/architecture/` - System design, data flows, technical deep-dives
2. **Monitoring docs** → `/monitoring/` - Prometheus, Grafana, alerts, dashboards
3. **Development docs** → `/development/` - Local setup, workflows, debugging, tools

### Documentation Standards

- Use Markdown format
- Include code examples where applicable
- Add diagrams for complex flows (use Mermaid or ASCII)
- Keep language clear and concise
- Update this README when adding new docs

## Additional Resources

- **Project Repository**: Main README.md in root directory
- **API Documentation**: See individual service READMEs
- **Issue Tracker**: GitHub Issues
- **Prometheus Docs**: https://prometheus.io/docs/
- **Grafana Docs**: https://grafana.com/docs/
- **NATS Docs**: https://docs.nats.io/
- **Go Docs**: https://go.dev/doc/
- **Node.js Docs**: https://nodejs.org/docs/
