# WS Server Refactoring Plan - Single/Multi Core Architecture

**Strategy:** Following ARCHITECTURAL_VARIANTS_STRATEGY.md
**Current Phase:** Refactoring single-core variant into proper structure
**Future:** Multi-core variant will be built separately in `internal/multi/`

## Target Directory Structure

```
ws_poc/
├── cmd/
│   └── ws-single/                    # Single-core binary (for now)
│       ├── main.go                   110 lines (move from ws/main.go)
│       └── Dockerfile                (new)
│
├── internal/                         # Private packages
│   ├── single/                       # 🎯 SINGLE-CORE SPECIFIC
│   │   ├── core/                     🔥 HOT PATH (performance-critical)
│   │   │   ├── server.go             ~300 lines (extracted from ws/server.go)
│   │   │   ├── broadcast.go          ~200 lines (extracted, 2-3% CPU)
│   │   │   ├── handlers.go           ~300 lines (extracted, 97% CPU)
│   │   │   ├── connection.go         454 lines (moved from ws/)
│   │   │   └── subscription.go       ~150 lines (extracted from connection.go)
│   │   │
│   │   ├── messaging/
│   │   │   ├── message.go            170 lines (moved from ws/)
│   │   │   └── protocol.go           ~250 lines (extracted from server.go)
│   │   │
│   │   ├── limits/
│   │   │   ├── resource_guard.go     409 lines (moved from ws/)
│   │   │   └── rate_limiter.go       264 lines (moved from ws/)
│   │   │
│   │   ├── monitoring/
│   │   │   ├── metrics.go            544 lines (moved from ws/)
│   │   │   ├── audit_logger.go       199 lines (moved from ws/)
│   │   │   ├── logger.go             180 lines (moved from ws/)
│   │   │   └── alerting.go           137 lines (moved from ws/)
│   │   │
│   │   ├── platform/
│   │   │   ├── config.go             188 lines (moved from ws/)
│   │   │   ├── cgroup.go             135 lines (moved from ws/)
│   │   │   └── cgroup_cpu.go         455 lines (moved from ws/)
│   │   │
│   │   └── kafka/
│   │       ├── consumer.go           304 lines (moved from ws/kafka/)
│   │       ├── config.go             95 lines (moved from ws/kafka/)
│   │       └── bundles.go            93 lines (moved from ws/kafka/)
│   │
│   ├── shared/                       # 🔄 SHARED (used by both single & multi)
│   │   └── (populated later when building multi-core variant)
│   │
│   └── multi/                        # 🚀 MULTI-CORE (future)
│       └── (created later)
│
├── pkg/                              # Public packages
│   ├── metrics/                      (future - truly independent metrics)
│   ├── logger/                       (future - standalone logger)
│   └── kafka/                        (future - reusable Kafka client)
│
└── go.mod                            # Single Go module
```

## Architectural Reasoning

### Why `internal/single/` instead of `internal/ws/`?

**From ARCHITECTURAL_VARIANTS_STRATEGY.md:**
> This is NOT version evolution (v1→v2), this is **architectural variants** (Design A vs Design B) that may coexist long-term.

We'll eventually have:
- **Single-core design (`internal/single/`)** - GOMAXPROCS=1, optimized for simplicity
- **Multi-core design (`internal/multi/`)** - GOMAXPROCS=N, optimized for parallelism
- **Shared code (`internal/shared/`)** - Common between both variants

### Migration Path

**Phase 1 (Now):** Build `internal/single/` from current `ws/`
```
ws/ → internal/single/
```

**Phase 2 (Later):** Build `internal/multi/` from scratch
```
New implementation: internal/multi/
Shared code: internal/shared/
```

**Phase 3 (Future):** Both coexist
```
cmd/ws-single/ → internal/single/
cmd/ws-multi/  → internal/multi/
Both use:      → internal/shared/
```

## Performance-Safe File Movement Plan

### Step 1: Move Cold Path (Low Risk)

**Platform Files (init only):**
```bash
git mv ws/cgroup.go internal/single/platform/
git mv ws/cgroup_cpu.go internal/single/platform/
git mv ws/config.go internal/single/platform/
```

**Monitoring Files (async):**
```bash
git mv ws/metrics.go internal/single/monitoring/
git mv ws/audit_logger.go internal/single/monitoring/
git mv ws/logger.go internal/single/monitoring/
git mv ws/alerting.go internal/single/monitoring/
```

**Limits Files (warm path, well-isolated):**
```bash
git mv ws/resource_guard.go internal/single/limits/
git mv ws/rate_limiter.go internal/single/limits/
```

**Kafka Files:**
```bash
git mv ws/kafka/consumer.go internal/single/kafka/
git mv ws/kafka/config.go internal/single/kafka/
git mv ws/kafka/bundles.go internal/single/kafka/
rmdir ws/kafka
```

**Messaging File:**
```bash
git mv ws/message.go internal/single/messaging/
```

**Connection File:**
```bash
git mv ws/connection.go internal/single/core/
```

### Step 2: Update Package Names

All moved files need package updates:

**Before:**
```go
package main  // or package kafka
```

**After:**
```go
package platform   // internal/single/platform/
package monitoring // internal/single/monitoring/
package limits     // internal/single/limits/
package kafka      // internal/single/kafka/
package messaging  // internal/single/messaging/
package core       // internal/single/core/
```

### Step 3: Create cmd/ws-single/main.go

```go
package main

import (
    "os"
    
    "github.com/adred-codev/ws_poc/internal/single/core"
    "github.com/adred-codev/ws_poc/internal/single/monitoring"
    "github.com/adred-codev/ws_poc/internal/single/platform"
)

func main() {
    // Load config
    logger := monitoring.InitLogger()
    config, err := platform.LoadConfig(&logger)
    if err != nil {
        logger.Fatal().Err(err).Msg("Failed to load config")
    }
    
    // Create and start server
    server, err := core.NewServer(*config)
    if err != nil {
        logger.Fatal().Err(err).Msg("Failed to create server")
    }
    
    if err := server.Start(); err != nil {
        logger.Fatal().Err(err).Msg("Server failed")
    }
}
```

### Step 4: Update server.go Imports

**Current ws/server.go imports:**
```go
import (
    "github.com/adred-codev/ws_poc/kafka"
)
```

**After moving to internal/single/core/server.go:**
```go
import (
    "github.com/adred-codev/ws_poc/internal/single/kafka"
    "github.com/adred-codev/ws_poc/internal/single/messaging"
    "github.com/adred-codev/ws_poc/internal/single/limits"
    "github.com/adred-codev/ws_poc/internal/single/monitoring"
    "github.com/adred-codev/ws_poc/internal/single/platform"
)
```

### Step 5: Update Dockerfile

**cmd/ws-single/Dockerfile:**
```dockerfile
FROM golang:1.25.1-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /ws-single ./cmd/ws-single

FROM alpine:latest
WORKDIR /root/
RUN apk --no-cache add ca-certificates
COPY --from=builder /ws-single .
EXPOSE 3002
CMD ["./ws-single"]
```

## Performance Testing After Each Step

**After Step 1 (file moves):**
```bash
go build -o /tmp/ws-single ./cmd/ws-single
# Should compile successfully
```

**After Step 2 (package renames):**
```bash
go build -o /tmp/ws-single ./cmd/ws-single
/tmp/ws-single &
curl http://localhost:3004/health
# Should start and serve health check
```

**After Step 3 (main.go created):**
```bash
# Full integration test
task gcp:deployment:rebuild:ws
task gcp:load-test:capacity:short
# Must match baseline: 3,850-4,000 connections
```

## Next Phases (After Directory Structure)

**Phase 3:** Extract broadcast.go (🔥 CRITICAL)
**Phase 4:** Extract handlers.go (🔥 CRITICAL - writePump)
**Phase 5:** Extract protocol.go
**Phase 6:** Final cleanup

See REFACTORING_BASELINE.md for performance budgets.
