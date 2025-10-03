# Taskfile Guide

Complete guide to using the Taskfile for streamlined development workflow.

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

List all available tasks:
```bash
task --list
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
