# Project Restructure Plan: Docker & Taskfile

## Current Structure Analysis

```
/src/                  # Go WebSocket server (production)
/node-server/          # Node.js WebSocket server (FULL SERVER + publisher inside)
  ├── server.ts        # WebSocket server implementation
  ├── publisher.ts     # NATS publisher (EXTRACT THIS)
  ├── config/          # Shared config (publisher needs this)
  ├── types/           # Shared types (publisher needs this)
  └── ... (domain, application, infrastructure)
/scripts/              # Testing scripts
/grafana/              # Grafana configs
/prometheus.yml        # Prometheus config
```

## Proposed New Structure

```
/src/                  # Go WebSocket server
/node-server/          # Node.js WebSocket server (KEEP AS IS - not Dockerized)
/publisher/            # Standalone NATS publisher (EXTRACTED)
  ├── publisher.ts
  ├── config/
  │   └── odin.config.ts
  ├── types/
  │   └── odin.types.ts
  ├── package.json
  ├── tsconfig.json
  ├── Dockerfile
  └── .env.example
/scripts/              # Testing & utility scripts
/docs/                 # All documentation (reorganized)
/grafana/              # Grafana provisioning
/docker-compose.yml    # Docker orchestration
/Taskfile.yml          # Task automation
/prometheus.yml        # Prometheus config
```

## Tasks to Complete

### 0. Extract Publisher from node-server
- Create `/publisher` directory
- Copy `node-server/publisher.ts` → `publisher/publisher.ts`
- Copy `node-server/config/odin.config.ts` → `publisher/config/odin.config.ts`
- Copy `node-server/types/odin.types.ts` → `publisher/types/odin.types.ts`
- Create `/publisher/package.json` (minimal dependencies: nats, express, cors, dotenv)
- Create `/publisher/tsconfig.json`
- Create `/publisher/.env.example`

### 1. Create Publisher Dockerfile
**File:** `/publisher/Dockerfile`

```dockerfile
# Stage 1: Build TypeScript
FROM node:18-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

# Stage 2: Runtime
FROM node:18-alpine
WORKDIR /app
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/package*.json ./
RUN npm ci --only=production
EXPOSE 3003
ENV NODE_ENV=production
CMD ["node", "dist/publisher.js"]
```

### 2. Update Go Server Dockerfile
**File:** `/src/Dockerfile`
- Change binary output: `/odin-ws-server-2` → `/odin-ws-server`
- Update CMD to use new binary name

### 3. Create docker-compose.yml

**Services:**
- `nats` - NATS 2.10-alpine
  - Ports: 4222 (client), 8222 (monitoring)
  - JetStream enabled
  - Health check
  - Resources: 1 CPU, 256MB RAM

- `ws-go` - Go WebSocket server (production)
  - Build: `./src`
  - Port: 3004:3002
  - Env: NATS_URL
  - Depends on: nats (healthy)
  - Resources: 2 CPU, 512MB RAM
  - ulimits: nofile 200000

- `publisher` - Node.js NATS publisher
  - Build: `./publisher`
  - Port: 3003:3003
  - Env: NATS_URL, PORT, NODE_ENV
  - Depends on: nats (healthy)
  - Resources: 0.5 CPU, 128MB RAM

- `prometheus` - Metrics collector
  - Image: prom/prometheus:latest
  - Port: 9091:9090
  - Volume: ./prometheus.yml
  - Scrape: ws-go:3002/metrics (15s interval)
  - Retention: 15 days
  - Resources: 1 CPU, 512MB RAM

- `grafana` - Visualization
  - Image: grafana/grafana:latest
  - Port: 3010:3000
  - Volume: ./grafana/provisioning
  - Env: admin/admin
  - Depends on: prometheus
  - Resources: 0.5 CPU, 256MB RAM

**Networks:** `odin-network` (bridge)
**Volumes:** `nats_data`, `prometheus_data`, `grafana_data`

### 4. Create Taskfile.yml

**Build Tasks:**
- `build:go` - Build Go binary locally
- `build:publisher` - Compile TypeScript publisher
- `build:docker` - Build all Docker images
- `build:docker:go` - Build just Go image
- `build:docker:publisher` - Build just publisher image

**Docker Management:**
- `docker:up` - Start all services (detached)
- `docker:down` - Stop & remove containers
- `docker:restart` - Restart all services
- `docker:rebuild` - Rebuild specific service (param: SERVICE)
- `docker:logs` - Tail all logs
- `docker:logs:go` - Go server logs only
- `docker:logs:publisher` - Publisher logs only
- `docker:logs:nats` - NATS logs only
- `docker:ps` - Show running containers
- `docker:clean` - Remove containers, volumes, orphans

**Development:**
- `dev:go` - Run Go server locally (outside Docker)
- `dev:publisher` - Run publisher locally
- `dev:nats` - Start only NATS container for local dev

**Testing:**
- `test:light` - 100 connections, 30s
- `test:medium` - 500 connections, 60s
- `test:heavy` - 2000 connections, 120s
- `test:custom` - Custom test (params: CONNECTIONS DURATION)

**Monitoring:**
- `monitor:health` - curl /health on all services
- `monitor:health:go` - curl Go server /health
- `monitor:metrics` - curl Go server /metrics
- `monitor:publisher:stats` - curl publisher /stats
- `monitor:grafana` - Open Grafana (http://localhost:3010)
- `monitor:prometheus` - Open Prometheus (http://localhost:9091)
- `monitor:targets` - Check Prometheus scrape targets

**Publisher Control:**
- `publisher:start` - Start publishing (POST /control action=start)
- `publisher:stop` - Stop publishing (POST /control action=stop)
- `publisher:configure` - Configure publisher (params)

**Utilities:**
- `install` - Install all dependencies (Go + Node)
- `install:go` - go mod download
- `install:publisher` - npm install in /publisher
- `format` - Format Go (gofmt) & TypeScript (prettier)
- `lint:go` - golangci-lint
- `lint:ts` - eslint publisher
- `clean` - Clean all build artifacts
- `clean:go` - Clean Go binaries
- `clean:publisher` - Clean publisher dist/

**Workflows (Combined Tasks):**
- `setup` - install → build:docker → docker:up
- `quick-start` - docker:up → test:light
- `full-test` - docker:up → test:medium → monitor:stats
- `dev` - docker:up nats → dev:go (terminal split for local dev)

### 5. Update Configuration Files

**prometheus.yml:**
- Update target: `ws-go-2:3002` → `ws-go:3002`
- Keep NATS scraping: `nats:8222`
- Keep 15s scrape interval

**grafana/provisioning/dashboards/websocket.json:**
- Update dashboard title: "WebSocket Server - Go-2" → "WebSocket Server - Go"
- Update descriptions (remove "Go-2" references)

### 6. Reorganize Documentation

**Create `/docs` structure:**
```
/docs/
  /architecture/
    - CONNECTION_LIMIT_EXPLAINED.md
    - FULL_RECONNECT_EXPLAINED.md
    - ARCHITECTURE_NATS_FLOW.md
    - CONNECTION_CLEANUP_EXPLAINED.md
    - REPLAY_MECHANISM_DEEP_DIVE.md
  /monitoring/
    - MONITORING_SETUP.md (from root MONITORING_README.md)
    - GRAFANA_DASHBOARDS.md
    - PROMETHEUS_QUERIES.md
  /development/
    - TASKFILE_GUIDE.md (how to use Taskfile)
    - LOCAL_DEVELOPMENT.md (running without Docker)
    - PUBLISHER_API.md (publisher control endpoints)
  README.md (overview, links to other docs)
```

**Move files:**
- `/src/docs/*.md` → `/docs/architecture/`
- Root `MONITORING_README.md` → `/docs/monitoring/MONITORING_SETUP.md` (if exists)
- Root `README.md` → Update with new structure

### 7. Update Root README.md

See template in plan (comprehensive quick start guide)

## Implementation Order

1. ✅ Extract publisher files to `/publisher`
2. ✅ Create `/publisher/package.json`
3. ✅ Create `/publisher/tsconfig.json`
4. ✅ Create `/publisher/Dockerfile`
5. ✅ Update `/src/Dockerfile` (binary name)
6. ✅ Create `docker-compose.yml`
7. ✅ Update `prometheus.yml`
8. ✅ Create `Taskfile.yml`
9. ✅ Reorganize `/docs` directory
10. ✅ Update root `README.md`
11. ✅ Test: `task build:docker`
12. ✅ Test: `task docker:up`
13. ✅ Test: `task monitor:health`
14. ✅ Test: `task test:light`

## Key Decisions

- `/node-server` stays as-is (not Dockerized per user request)
- `/publisher` is standalone, extracted from `/node-server`
- `/src` is the production Go server
- All docs consolidated in `/docs` with clear structure
- Taskfile provides streamlined workflow

## Service URLs (After Implementation)

| Service | URL | Description |
|---------|-----|-------------|
| Go WebSocket | ws://localhost:3004/ws | Production WebSocket server |
| Go Health | http://localhost:3004/health | Health check endpoint |
| Go Metrics | http://localhost:3004/metrics | Prometheus metrics |
| Publisher API | http://localhost:3003/control | Publisher control API |
| Publisher Stats | http://localhost:3003/stats | Publisher statistics |
| Grafana | http://localhost:3010 | Dashboards (admin/admin) |
| Prometheus | http://localhost:9091 | Metrics database |
| NATS | nats://localhost:4222 | NATS message broker |
