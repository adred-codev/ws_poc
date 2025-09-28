# 🚀 Odin WebSocket - Task-Based Development Workflow

**Modern Task Runner for Node.js and Go WebSocket Servers**

This project now uses **Taskfile.dev** for all development, testing, and deployment tasks, replacing npm scripts and Makefiles with a unified, cross-platform task runner.

## 📋 **Quick Reference**

### **Essential Commands**
```bash
# Quick Start
task start              # Start Node.js stack (NATS + Server + Publisher + Client)
task start:go          # Start Go stack (NATS + Go Server + Publisher + Client)
task start:both        # Start both servers for comparison

# Development
task dev               # Development mode (Node.js)
task dev:go           # Development mode (Go)
task dev:compare      # Both servers for comparison

# Testing
task test             # Quick load test on available server
task test:all         # Comprehensive performance comparison

# Management
task status           # Check all service status
task stop             # Stop all services
task clean            # Clean up and stop everything
```

### **Get Help**
```bash
task help             # Show main commands
task --list           # Show all available tasks
task <module>:help    # Help for specific module (if available)
```

## 🏗️ **Task Architecture**

### **Modular Task Structure**
```
Taskfile.yml (root orchestrator)
├── tasks/nats.yml      # NATS server management
├── tasks/nodejs.yml    # Node.js servers & utilities
├── tasks/go.yml        # Go server & build tools
├── tasks/client.yml    # Web client management
├── tasks/metrics.yml   # Unified metrics collection
├── tasks/test.yml      # Load testing & comparison
└── tasks/dev.yml       # Development workflows
```

### **Cross-Platform Benefits**
- ✅ **Works everywhere**: macOS, Linux, Windows
- ✅ **No Make dependency**: Pure Go binary
- ✅ **Better error handling**: Built-in retry and validation
- ✅ **Parallel execution**: Run multiple tasks concurrently
- ✅ **Environment variables**: Advanced configuration support

## 🚀 **Complete Workflows**

### **1. Node.js Development Stack**
```bash
task start
```
**What it does:**
- Starts NATS server with Docker
- Launches Node.js WebSocket server (port 3001)
- Starts price publisher for data generation
- Serves web client (port 8080)
- Opens metrics dashboard automatically

### **2. Go Development Stack**
```bash
task start:go
```
**What it does:**
- Starts NATS server with Docker
- Builds and launches Go WebSocket server (port 3002)
- Starts Node.js publisher for data generation
- Serves web client (port 8080)
- Opens Prometheus metrics

### **3. Performance Comparison**
```bash
task start:both
```
**What it does:**
- Runs both Node.js and Go servers simultaneously
- Provides side-by-side metrics comparison
- Enables A/B testing and benchmarking

## 📊 **Metrics & Monitoring**

### **Unified Metrics Collection**
```bash
# View current metrics
task metrics:node:current      # Node.js metrics
task metrics:go:current        # Go server metrics
task metrics:compare:side-by-side  # Both dashboards

# Open dashboards
task metrics:node:dashboard    # Node.js dashboard
task metrics:go:prometheus     # Go Prometheus metrics
task metrics:go:stats         # Go stats page

# Collect for analysis
task metrics:collect:all      # Save current metrics
task metrics:generate:report  # Generate comparison report
```

### **Real-time Monitoring**
```bash
task metrics:watch:node       # Real-time Node.js metrics
task metrics:watch:go         # Real-time Go metrics
```

## 🧪 **Testing & Benchmarking**

### **Load Testing Options**
```bash
# Quick tests (100 connections)
task test:quick:node          # Test Node.js server
task test:quick:go            # Test Go server
task test:quick               # Test available server

# Comprehensive testing
task test:medium:node         # 1000 connections
task test:stress:go           # 5000 connections
task test:progressive:node    # 100→1K→5K progression
```

### **Performance Comparison**
```bash
# Quick comparison
task test:compare:quick       # Quick Node.js vs Go test

# Comprehensive comparison
task test:compare:performance # Full testing with metrics

# Stress testing comparison
task test:compare:stress      # High-load comparison
```

## 🌐 **Client Management**

### **Web Client Control**
```bash
# Start client server
task client:serve             # Serve on port 8080

# Open connected to specific server
task client:open:node         # Connect to Node.js server
task client:open:go           # Connect to Go server

# Test connectivity
task client:test:node         # Test Node.js connection
task client:test:go           # Test Go connection
```

### **Client Configuration**
```bash
# Update default server
task client:update:node-config    # Default to Node.js
task client:update:go-config      # Default to Go server
task client:restore:config        # Restore original
```

## ⚙️ **Development Tools**

### **Code Quality**
```bash
# Node.js/TypeScript
task node:lint                # ESLint checking
task node:lint:fix            # Auto-fix issues
task node:format              # Prettier formatting
task node:typecheck           # TypeScript validation

# Go
task go:fmt                   # Format Go code
task go:lint                  # Go linting
task go:test                  # Run Go tests
task go:test:race            # Race condition detection

# All languages
task dev:lint:all            # Lint everything
task dev:format:all          # Format everything
task dev:quality:all         # All quality checks
```

### **Build & Development**
```bash
# Go builds
task go:build                # Build for current platform
task go:build:all           # Multi-platform builds
task go:clean               # Clean build artifacts

# Development setup
task dev:setup              # Initial project setup
task dev:reset              # Clean reset everything
task dev:check              # Health check
task dev:install:tools      # Install dev tools
```

## 🐳 **Docker & Deployment**

### **NATS Management**
```bash
task nats:start             # Start NATS server
task nats:stop              # Stop NATS server
task nats:restart           # Restart NATS
task nats:logs              # View NATS logs
task nats:monitor           # Open monitoring dashboard
task nats:status            # Check NATS status
```

### **Docker Operations**
```bash
# Go server Docker
task go:docker:build        # Build Docker image
task go:docker:run          # Run container
task go:docker:stop         # Stop container

# NATS Docker (via docker-compose)
task nats:start             # Uses docker-compose
task nats:clean             # Remove containers/data
```

## 🔧 **Advanced Usage**

### **Environment Variables**
```bash
# Override default ports
NODE_SERVER_PORT=3005 task start
GO_SERVER_PORT=3006 task start:go
CLIENT_PORT=9000 task client:serve

# Custom NATS URL
NATS_URL=nats://remote-host:4222 task start:go
```

### **Configuration**
```bash
# Use custom Go config
task go:dev -config=/path/to/config.json

# Custom metrics directory
METRICS_DIR=./custom-metrics task metrics:collect:all
```

### **Parallel Task Execution**
```bash
# Run multiple tasks in parallel (if Taskfile supports it)
task --parallel nats:start go:build node:publisher
```

## 📈 **Performance Benefits**

### **Node.js vs Task Runner**
```bash
# Old way (npm scripts)
npm run docker:up && npm run odin:server && npm run odin:publisher

# New way (Task)
task start
```

### **Makefile vs Taskfile**
- ✅ **Cross-platform**: No Make dependency on Windows
- ✅ **Better error handling**: Built-in validation
- ✅ **YAML syntax**: More readable than Makefile
- ✅ **Dependency management**: Advanced task dependencies
- ✅ **Variable support**: Environment and task variables

## 🚨 **Troubleshooting**

### **Common Issues**
```bash
# Task not found
task --list                 # Show all available tasks

# Service won't start
task status                 # Check what's running
task stop && task clean     # Clean restart

# Port conflicts
task stop                   # Stop everything
lsof -i :3001              # Check what's using ports
task start                 # Restart
```

### **Health Checks**
```bash
task dev:check              # Full system health check
task connectivity           # Test all connections
task nats:status            # Check NATS specifically
```

### **Reset Everything**
```bash
task dev:reset              # Nuclear option - reset everything
task clean                  # Clean up without restart
```

## 🔄 **Migration from npm/Make**

### **Old Commands → New Commands**
```bash
# Server management
npm run odin:server        → task node:server
npm run odin:publisher     → task node:publisher
npm run docker:up          → task nats:start

# Development
npm run dev                → task dev
npm run typecheck          → task node:typecheck
npm run lint               → task node:lint

# Go (Makefile)
make build                 → task go:build
make dev                   → task go:dev
make clean                 → task go:clean
make load-test-go          → task test:quick:go

# Combined workflows
(multiple commands)        → task start
(multiple commands)        → task start:go
```

## 📚 **Additional Resources**

- **Task Documentation**: https://taskfile.dev/
- **Go Server README**: `go-server/README.md`
- **Load Testing Guide**: `LOAD_TESTING.md`
- **Project README**: `README.md`

---

## 🎯 **Quick Start Summary**

```bash
# 1. Install Task (if not already installed)
brew install go-task/tap/go-task     # macOS
# or see: https://taskfile.dev/installation/

# 2. Initial setup
task dev:setup

# 3. Start Node.js stack
task start

# 4. Or start Go stack
task start:go

# 5. Run performance comparison
task test:compare:quick

# 6. View all available commands
task --list
```

**🚀 Production-ready task-based development workflow with unified Node.js and Go server management!**