# Taskfile Modules

This directory contains modular Taskfiles for the Odin WebSocket Server project.

## Structure

```
taskfiles/
├── build.yml         - Build tasks (Go server, TypeScript publisher, Docker)
├── docker.yml        - Docker container management
├── dev.yml           - Local development tasks
├── test.yml          - Stress testing and load testing
├── monitor.yml       - Health checks, metrics, and monitoring
├── publisher.yml     - Publisher control (start/stop/configure)
├── utils.yml         - Utilities (install, format, clean)
├── deploy-gcp.yml    - GCP deployment and management
└── README.md         - This file
```

## Usage

All tasks are namespaced by their module. For example:

```bash
# Build tasks
task build:go
task build:docker
task build:all

# Docker tasks
task docker:up
task docker:down
task docker:logs

# GCP deployment
task gcp:setup
task gcp:create-vm
task gcp:deploy:initial

# Testing
task test:light
task test:medium
task test:realistic
```

## Quick Start

```bash
# List all available tasks
task --list

# List all tasks (including subtasks)
task --list-all

# View help for specific module
task build: --list
task gcp: --list
```

## Combined Workflows

The main Taskfile provides convenient shortcuts:

- `task setup` - Complete local setup (install → build → start)
- `task quick-start` - Quick start for testing
- `task full-test` - Full test workflow with health checks

## GCP Deployment

The `deploy-gcp.yml` module converts the deployment documentation into executable tasks:

### Phase 1: Deploy to Single VM

```bash
# 1. Setup GCP environment
task gcp:setup

# 2. Enable APIs and create firewall rules
task gcp:enable-apis
task gcp:firewall

# 3. Create VM instance
task gcp:create-vm

# 4. Reserve static IP (optional but recommended)
task gcp:reserve-ip

# 5. Initial deployment (follow prompts)
task gcp:deploy:initial

# 6. Setup auto-start service
task gcp:systemd
```

### Or use the combined workflow:

```bash
task gcp:full-deploy
```

### Operations

```bash
# Get IP address
task gcp:ip

# Check health
task gcp:health

# View logs
task gcp:logs

# Update deployment
task gcp:deploy:update

# Restart services
task gcp:restart

# Stop VM (save money)
task gcp:stop

# Start VM
task gcp:start

# Create backup
task gcp:backup
```

## Environment Variables

GCP tasks support environment variable overrides:

```bash
# Custom project
export GCP_PROJECT_ID=my-project
task gcp:setup

# Custom region/zone
export GCP_REGION=us-west1
export GCP_ZONE=us-west1-a
task gcp:create-vm

# Custom machine type
export GCP_MACHINE_TYPE=e2-standard-2
task gcp:create-vm
```

## Adding New Tasks

1. Choose the appropriate module file (or create a new one)
2. Add your task following the existing pattern
3. Update this README if creating a new module
4. Test with `task --list-all` to verify it appears

## Module Guidelines

- **Keep modules focused** - Each file should handle one domain
- **Use descriptive names** - Task names should be self-explanatory
- **Add descriptions** - Every task should have a `desc` field
- **Document parameters** - If task uses vars, document in task description
- **Test thoroughly** - Verify tasks work in isolation and combination
