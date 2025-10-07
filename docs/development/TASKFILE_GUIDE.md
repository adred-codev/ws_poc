# Taskfile Guide

Complete guide to using the Taskfile for streamlined development workflow.

## 🏗️ Modular Structure

The project uses a **modular Taskfile structure** for better organization and maintainability:

```
Taskfile.yml                 # Main orchestrator
taskfiles/
├── build.yml               # Build tasks
├── docker.yml              # Docker management
├── dev.yml                 # Development tasks
├── test.yml                # Testing tasks
├── monitor.yml             # Monitoring & health checks
├── publisher.yml           # Publisher control
├── utils.yml               # Utilities (install, format, clean)
├── deploy-gcp.yml          # GCP deployment (360 lines of automation!)
└── README.md               # Module documentation
```

**Benefits:**
- ✅ Better organization (~50-80 lines per module vs 383-line monolith)
- ✅ Clear namespacing (`build:`, `docker:`, `gcp:`, etc.)
- ✅ Easy to find and modify tasks
- ✅ Reduced merge conflicts

**All tasks are namespaced by module:**
```bash
task build:go           # From taskfiles/build.yml
task docker:up          # From taskfiles/docker.yml
task gcp:deploy:initial # From taskfiles/deploy-gcp.yml
```

See [taskfiles/README.md](../../taskfiles/README.md) for module details.

## Prerequisites

Install Task:
```bash
# macOS
brew install go-task

# Linux
sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b ~/.local/bin

# Or via npm
npm install -g @go-task/cli
```

## Quick Reference

```bash
# List top-level tasks
task --list

# List ALL tasks (including subtasks)
task --list-all

# View tasks by module
task build:      # Shows: build:go, build:docker, etc.
task docker:     # Shows: docker:up, docker:down, etc.
task gcp:        # Shows: gcp:setup, gcp:deploy:initial, etc.
```

## Build Tasks

### `task build:go`
Build Go server binary locally in `/src`

```bash
task build:go
```

### `task build:publisher`
Compile TypeScript publisher to JavaScript

```bash
task build:publisher
```

### `task build:docker`
Build all Docker images (ws-go + publisher)

```bash
task build:docker
```

### `task build:docker:go`
Build only Go server Docker image

```bash
task build:docker:go
```

### `task build:docker:publisher`
Build only publisher Docker image

```bash
task build:docker:publisher
```

### `task build:all`
Build everything (local binaries + Docker images)

```bash
task build:all
```

## Docker Management

### `task docker:up`
Start all services in detached mode

```bash
task docker:up
```

Starts: NATS, Go WebSocket server, Publisher, Prometheus, Grafana

### `task docker:down`
Stop and remove all containers

```bash
task docker:down
```

### `task docker:restart`
Restart all services (down then up)

```bash
task docker:restart
```

### `task docker:rebuild`
Rebuild and restart a specific service

```bash
task docker:rebuild SERVICE=ws-go
task docker:rebuild SERVICE=publisher
```

### `task docker:logs`
Tail all service logs

```bash
task docker:logs
```

### `task docker:logs:go`
Tail only Go server logs

```bash
task docker:logs:go
```

### `task docker:logs:publisher`
Tail only publisher logs

```bash
task docker:logs:publisher
```

### `task docker:ps`
Show running containers

```bash
task docker:ps
```

### `task docker:clean`
Remove all containers, volumes, and orphans

```bash
task docker:clean
```

## Development

### `task dev:go`
Run Go server locally (outside Docker)

```bash
task dev:go
```

Requires NATS running on `localhost:4222`

### `task dev:publisher`
Run publisher locally

```bash
task dev:publisher
```

### `task dev:nats`
Start only NATS container for local development

```bash
task dev:nats
```

**Local Development Workflow:**
```bash
# Terminal 1: Start NATS
task dev:nats

# Terminal 2: Run Go server locally
task dev:go

# Terminal 3: Run publisher locally
task dev:publisher
```

## Testing

### `task test:light`
Run light stress test (100 connections, 30 seconds)

```bash
task test:light
```

### `task test:medium`
Run medium stress test (500 connections, 60 seconds)

```bash
task test:medium
```

### `task test:heavy`
Run heavy stress test (2000 connections, 120 seconds)

```bash
task test:heavy
```

### `task test:custom`
Run custom stress test with parameters

```bash
task test:custom CONNECTIONS=1000 DURATION=45
```

## Monitoring

### `task monitor:health`
Check health of all services

```bash
task monitor:health
```

### `task monitor:health:go`
Check only Go server health

```bash
task monitor:health:go
```

### `task monitor:metrics`
View Go server Prometheus metrics

```bash
task monitor:metrics
```

### `task monitor:publisher:stats`
View publisher statistics

```bash
task monitor:publisher:stats
```

### `task monitor:grafana`
Open Grafana dashboard in browser

```bash
task monitor:grafana
```

### `task monitor:prometheus`
Open Prometheus UI in browser

```bash
task monitor:prometheus
```

### `task monitor:targets`
Check Prometheus scrape targets status

```bash
task monitor:targets
```

## Publisher Control

### `task publisher:start`
Start publishing messages

```bash
task publisher:start
```

### `task publisher:stop`
Stop publishing messages

```bash
task publisher:stop
```

### `task publisher:configure`
Configure publisher message rate

```bash
task publisher:configure RATE=50
```

## Utilities

### `task install`
Install all dependencies (Go + Node.js)

```bash
task install
```

### `task install:go`
Install only Go dependencies

```bash
task install:go
```

### `task install:publisher`
Install only publisher Node.js dependencies

```bash
task install:publisher
```

### `task format`
Format all code (Go + TypeScript)

```bash
task format
```

### `task format:go`
Format only Go code

```bash
task format:go
```

### `task format:ts`
Format only TypeScript code

```bash
task format:ts
```

### `task clean`
Clean all build artifacts

```bash
task clean
```

### `task clean:go`
Clean only Go build artifacts

```bash
task clean:go
```

### `task clean:publisher`
Clean only publisher build artifacts

```bash
task clean:publisher
```

## Combined Workflows

### `task setup`
Complete setup from scratch

```bash
task setup
```

Runs: install → build:docker → docker:up

### `task quick-start`
Quick start for testing

```bash
task quick-start
```

Runs: docker:up → wait 5s → test:light

### `task full-test`
Full testing workflow

```bash
task full-test
```

Runs: docker:up → wait 5s → test:medium → monitor:health → monitor:publisher:stats

## Common Workflows

### First Time Setup
```bash
task setup
```

### Daily Development
```bash
# Start services
task docker:up

# Make changes to code...

# Rebuild and restart specific service
task docker:rebuild SERVICE=ws-go

# View logs
task docker:logs:go
```

### Testing Changes
```bash
# Build and start
task docker:up

# Run tests
task test:medium

# Check metrics
task monitor:grafana
```

### Clean Restart
```bash
task docker:clean
task docker:up
```

### Local Development (No Docker)
```bash
# Terminal 1: NATS only
task dev:nats

# Terminal 2: Go server
task dev:go

# Terminal 3: Publisher
task dev:publisher

# Terminal 4: Run tests
task test:light
```

## GCP Deployment Tasks

**New!** Complete GCP deployment automation in `taskfiles/deploy-gcp.yml` (360 lines).

### Infrastructure Setup

```bash
task gcp:setup           # Configure gcloud CLI and authenticate
task gcp:enable-apis     # Enable required GCP APIs
task gcp:firewall        # Create firewall rules
task gcp:create-vm       # Create VM instance with Docker
task gcp:reserve-ip      # Reserve and assign static IP
```

### Deployment

```bash
task gcp:deploy:initial  # First-time deployment (interactive)
task gcp:deploy:update   # Update existing deployment
task gcp:systemd         # Setup auto-start service
```

### Operations

```bash
task gcp:ssh             # SSH into GCP instance
task gcp:ip              # Get external IP address
task gcp:health          # Check deployment health
task gcp:logs            # View application logs
task gcp:logs:tail       # View last 100 lines
task gcp:restart         # Restart services via systemd
```

### Docker Management

```bash
task gcp:docker:ps       # Show running containers on GCP instance
task gcp:docker:up       # Start all Docker services
task gcp:docker:down     # Stop all Docker services
task gcp:docker:restart  # Restart all Docker services
```

### Publisher Control

```bash
task gcp:publisher:start              # Start publisher (default 10 msgs/sec)
task gcp:publisher:start RATE=20      # Start with specific rate
task gcp:publisher:stop               # Stop publisher
task gcp:publisher:configure RATE=15  # Change message rate
task gcp:publisher:stats              # View publisher statistics
```

### VM Lifecycle

```bash
task gcp:start           # Start VM instance
task gcp:stop            # Stop VM (save money)
task gcp:status          # Show VM status
task gcp:delete          # Delete VM (WARNING - destructive!)
```

### Maintenance

```bash
task gcp:backup          # Create Prometheus/Grafana backups
```

### Combined Workflows

```bash
task gcp:full-deploy     # Complete deployment (setup → create → deploy)
task gcp:quick-check     # Quick health and status check
```

### Environment Variable Overrides

```bash
# Custom project/region/zone
export GCP_PROJECT_ID=my-project
export GCP_REGION=us-west1
export GCP_ZONE=us-west1-a

# Custom machine type
export GCP_MACHINE_TYPE=e2-standard-2
```

See [GCP Deployment Guide](../deployment/GCP_DEPLOYMENT.md) for detailed instructions and [taskfiles/README.md](../../taskfiles/README.md) for implementation details.

---

## Tips

1. **Tab Completion**: Task supports shell completion. See [Task docs](https://taskfile.dev/installation/)

2. **Parallel Execution**: Task runs independent tasks in parallel by default

3. **Watch Mode**: Add `-w` to watch files and re-run on changes
   ```bash
   task -w build:go
   ```

4. **Verbose Output**: Add `-v` for verbose output
   ```bash
   task -v docker:up
   ```

5. **Dry Run**: Add `--dry` to see what would be executed
   ```bash
   task --dry build:all
   ```
