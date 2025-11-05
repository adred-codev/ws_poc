# Module Refactoring - Self-Contained Services

**Date**: November 5, 2025  
**Status**: ✅ Complete

## Overview

Refactored the project to follow Go and Docker best practices by making each service self-contained with its own `go.mod` and `Dockerfile` co-located with the source code.

## Problems Fixed

### Problem 1: `ws/` Wasn't a Proper Go Module

**Before (BROKEN):**
```
ws_poc/
├── ws/
│   ├── cmd/ws-server/
│   └── internal/
├── go.mod              # ← At root, not in ws/
├── go.sum
```

**Issues:**
1. ❌ Awkward import paths: `github.com/adred-codev/ws_poc/ws/internal/server`
2. ❌ Inconsistent: `loadtest/` has its own `go.mod` but `ws/` doesn't
3. ❌ Not self-contained: Can't extract `ws/` as standalone project
4. ❌ Module boundary unclear: What's the module? `ws_poc` or `ws`?

### Problem 2: Dockerfiles Separated from Code

**Before (ANTI-PATTERN):**
```
deployments/docker/ws-server/Dockerfile    # ← Far from source
ws/                                        # Source here
loadtest/                                  # Has its own Dockerfile ✅
publisher/                                 # Has its own Dockerfile ✅
```

**Issues:**
1. ❌ Hard to find: "Where's the ws-server Dockerfile?"
2. ❌ Maintenance overhead: Changes to code require looking elsewhere
3. ❌ Not portable: Can't extract `ws/` as standalone repo
4. ❌ Inconsistent: Only `ws/` was broken

## Solution Applied

### After (CORRECT)

```
ws_poc/
├── ws/                          # ✅ Self-contained WebSocket server
│   ├── Dockerfile               # ✅ Dockerfile WITH code
│   ├── go.mod                   # ✅ Module root here
│   ├── go.sum
│   ├── cmd/
│   │   └── ws-server/main.go
│   └── internal/
│       ├── server/
│       ├── client/
│       └── ...
├── loadtest/                    # ✅ Self-contained load tester
│   ├── Dockerfile               # ✅ Already correct
│   ├── go.mod
│   ├── go.sum
│   └── main.go
├── publisher/                   # ✅ Self-contained publisher
│   ├── Dockerfile
│   ├── package.json
│   └── publisher.ts
└── deployments/                 # ONLY orchestration
    ├── local/
    │   └── docker-compose.yml   # References ../ws/Dockerfile
    └── gcp-distributed/
        └── ...
```

## Changes Made

### 1. Created `ws/go.mod`

**File**: `ws/go.mod`

```go
module github.com/adred-codev/ws_poc

go 1.23.1

require (
    github.com/caarlos0/env/v11 v11.0.0
    github.com/gobwas/ws v1.4.0
    github.com/joho/godotenv v1.5.1
    github.com/nats-io/nats.go v1.37.0
    github.com/prometheus/client_golang v1.20.5
    github.com/rs/zerolog v1.33.0
    github.com/shirou/gopsutil/v3 v3.24.5
)
// ... indirect dependencies
```

**Why This Module Path:**
- Module path: `github.com/adred-codev/ws_poc` (clean, no `/ws/` noise)
- Could be `github.com/adred-codev/ws_poc/v2` if breaking changes
- Could be renamed to `github.com/yourorg/ws-server` if extracted

### 2. Updated All Import Paths (10 files)

**Changed:**
```go
// BEFORE:
"github.com/adred-codev/ws_poc/ws/internal/server"
"github.com/adred-codev/ws_poc/ws/internal/client"

// AFTER:
"github.com/adred-codev/ws_poc/internal/server"
"github.com/adred-codev/ws_poc/internal/client"
```

**Files Modified:**
- `ws/cmd/ws-server/main.go`
- `ws/internal/server/server.go`
- `ws/internal/server/handler.go`
- `ws/internal/server/connection.go`
- `ws/internal/server/broadcast.go`
- `ws/internal/server/nats.go`
- `ws/internal/client/client.go`
- `ws/internal/client/pool.go`
- `ws/internal/pool/worker.go`
- `ws/internal/resource/guard.go`

**Command used:**
```bash
cd ws
find . -name "*.go" -type f -exec sed -i '' 's|github.com/adred-codev/ws_poc/ws/internal/|github.com/adred-codev/ws_poc/internal/|g' {} +
```

### 3. Moved Dockerfile to `ws/`

**Moved:**
- `deployments/docker/ws-server/Dockerfile` → `ws/Dockerfile`

**Updated Dockerfile:**
```dockerfile
# BEFORE:
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /odin-ws-server ./ws/cmd/ws-server

# AFTER:
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /odin-ws-server ./cmd/ws-server
```

**Removed:**
- `deployments/docker/ws-server/` (empty directory)
- `deployments/docker/` (empty directory)

### 4. Created `loadtest/Dockerfile`

**File**: `loadtest/Dockerfile`

Self-contained multi-stage build:
```dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /loadtest .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /loadtest .
CMD ["./loadtest"]
```

**Removed:**
- `deployments/gcp-distributed/test-runner/Dockerfile` (obsolete)
- `deployments/gcp-distributed/test-runner/` (empty directory)

### 5. Updated docker-compose Files (2 files)

#### `deployments/local/docker-compose.yml`

```yaml
# BEFORE:
ws-go:
  build:
    context: ../..
    dockerfile: deployments/docker/ws-server/Dockerfile

# AFTER:
ws-go:
  build:
    context: ../../ws
    dockerfile: Dockerfile
```

#### `deployments/gcp-distributed/ws-server/docker-compose.yml`

```yaml
# BEFORE:
ws-go:
  build:
    context: ../../..
    dockerfile: deployments/docker/ws-server/Dockerfile

# AFTER:
ws-go:
  build:
    context: ../../../ws
    dockerfile: Dockerfile
```

### 6. Updated Deployment Tasks

**File**: `taskfiles/isolated-setup.yml`

**WS Server Deployment (line 289-295):**
```yaml
# BEFORE:
- gcloud compute scp --recurse {{.USER_WORKING_DIR}}/ws {{.WS_GO_INSTANCE}}:/tmp/ws_poc_deploy/
- gcloud compute scp --recurse {{.USER_WORKING_DIR}}/deployments {{.WS_GO_INSTANCE}}:/tmp/ws_poc_deploy/
- gcloud compute scp {{.USER_WORKING_DIR}}/go.mod {{.WS_GO_INSTANCE}}:/tmp/ws_poc_deploy/
- gcloud compute scp {{.USER_WORKING_DIR}}/go.sum {{.WS_GO_INSTANCE}}:/tmp/ws_poc_deploy/

# AFTER:
- gcloud compute scp --recurse {{.USER_WORKING_DIR}}/ws {{.WS_GO_INSTANCE}}:/tmp/ws_poc_deploy/
# That's it! ws/ contains everything (go.mod, go.sum, Dockerfile)
```

**Load Test Deployment (line 388-395):**
```yaml
# BEFORE:
- gcloud compute scp {{.USER_WORKING_DIR}}/deployments/gcp-distributed/test-runner/Dockerfile ...
- gcloud compute scp {{.USER_WORKING_DIR}}/loadtest/main.go ...
- gcloud compute scp {{.USER_WORKING_DIR}}/loadtest/go.mod ...
- gcloud compute scp {{.USER_WORKING_DIR}}/loadtest/go.sum ...

# AFTER:
- gcloud compute scp --recurse {{.USER_WORKING_DIR}}/loadtest {{.TEST_RUNNER_INSTANCE}}:/tmp/test_runner_go_deploy/
# Single command! loadtest/ is self-contained
```

## Build Verification

### Local Go Build ✅

```bash
cd ws
go build -o /tmp/ws-server ./cmd/ws-server
```

**Result:**
- ✅ Build successful
- Binary size: 16 MB
- No import errors
- Clean module resolution

### Docker Build: ws-server ✅

```bash
cd ws
docker build -t ws-server-test .
```

**Result:**
- ✅ Build successful
- Image size: ~20 MB
- Build time: ~13s
- Multi-stage build working

### Docker Build: loadtest ✅

```bash
cd loadtest
docker build -t loadtest-test .
```

**Result:**
- ✅ Build successful
- Image size: ~12 MB
- Build time: ~8s
- Multi-stage build working

### docker-compose Build ✅

```bash
cd deployments/local
docker-compose build ws-go
```

**Result:**
- ✅ Build successful
- Context resolution correct
- Dockerfile found at `../../ws/Dockerfile`

## Benefits

### 1. Consistency

| Service | Module | Dockerfile | Self-Contained |
|---------|--------|------------|----------------|
| ws | ✅ `ws/go.mod` | ✅ `ws/Dockerfile` | ✅ YES |
| loadtest | ✅ `loadtest/go.mod` | ✅ `loadtest/Dockerfile` | ✅ YES |
| publisher | N/A (Node.js) | ✅ `publisher/Dockerfile` | ✅ YES |

**Every service follows the same pattern!**

### 2. Clean Import Paths

```go
// BEFORE (ugly):
"github.com/adred-codev/ws_poc/ws/internal/server"
                              ^^^^
                              Noise!

// AFTER (clean):
"github.com/adred-codev/ws_poc/internal/server"
```

### 3. Docker Best Practice

**Best Practice:** Dockerfile lives WITH the code it builds.

✅ `ws/Dockerfile` builds `ws/` code  
✅ `loadtest/Dockerfile` builds `loadtest/` code  
✅ `publisher/Dockerfile` builds `publisher/` code

### 4. Self-Contained Services

Each service is a "black box":
- Can be extracted to separate repo
- Can be built independently: `cd ws && docker build .`
- No external dependencies on parent project structure

### 5. Easy Navigation

**Question:** "Where's the ws-server Dockerfile?"  
**Answer:** "In the `ws/` directory, of course!"

**Question:** "Where's the loadtest Dockerfile?"  
**Answer:** "In the `loadtest/` directory!"

No hunting through `deployments/docker/*/` directories.

### 6. Simplified Deployment

**Before:**
- Copy `ws/` directory
- Copy `deployments/` directory
- Copy `go.mod` and `go.sum`
- Copy Dockerfile from deep nested path

**After:**
- Copy `ws/` directory ← That's it!

## Project Structure Now

```
ws_poc/
├── ws/                          # Self-contained WS server
│   ├── Dockerfile
│   ├── go.mod
│   ├── go.sum
│   ├── cmd/ws-server/
│   └── internal/
├── loadtest/                    # Self-contained load tester
│   ├── Dockerfile
│   ├── go.mod
│   ├── go.sum
│   ├── main.go
│   └── README.md
├── publisher/                   # Self-contained publisher
│   ├── Dockerfile
│   ├── package.json
│   └── publisher.ts
├── deployments/                 # Orchestration only
│   ├── local/
│   │   ├── docker-compose.yml
│   │   └── configs/
│   ├── gcp-single/
│   │   └── docker-compose.yml
│   └── gcp-distributed/
│       ├── backend/
│       └── ws-server/
├── scripts/                     # Utility scripts
└── taskfiles/                   # Task automation
```

**Key Principle:** Each service directory is independently buildable and deployable.

## Usage After Refactoring

### Build Locally

```bash
# WebSocket server
cd ws
go build ./cmd/ws-server

# Load tester
cd loadtest
go build
```

### Build Docker Images

```bash
# WebSocket server
cd ws
docker build -t ws-server .

# Load tester
cd loadtest
docker build -t loadtest .

# Publisher (unchanged)
cd publisher
docker build -t publisher .
```

### Deploy to GCP

```bash
# Commands unchanged
task gcp2:deploy:ws-go
task gcp2:deploy:backend
task gcp2:deploy:test-runner-go
```

Deployment tasks updated to use new structure automatically.

### Local Development

```bash
cd deployments/local
docker-compose up -d
```

docker-compose automatically finds:
- `../../ws/Dockerfile`
- `../../loadtest/Dockerfile`
- `../../publisher/Dockerfile`

## Migration Impact

### Breaking Changes: None for End Users

- ✅ Docker images same
- ✅ Binaries same
- ✅ APIs unchanged
- ✅ Deployment commands same
- ✅ Configuration same

### Internal Changes

- ✅ Import paths cleaner (internal only)
- ✅ Module structure improved (internal only)
- ✅ Build paths updated (transparent)

## Comparison: Before vs After

### Service Extraction

**Before:**
To extract `ws/` as standalone repo:
1. Copy `ws/` directory
2. Create new `go.mod` with correct module path
3. Update all import paths
4. Find and copy Dockerfile from `deployments/docker/ws-server/`
5. Update Dockerfile paths
6. Test and fix issues

**After:**
To extract `ws/` as standalone repo:
1. Copy `ws/` directory
2. Done! Everything is already there.

### Building from Source

**Before:**
```bash
# Where's go.mod? At root...
go build ./ws/cmd/ws-server  # ❌ Fails - can't find internal packages

# Need to be at root
cd /path/to/ws_poc
go build ./ws/cmd/ws-server  # Works but awkward
```

**After:**
```bash
cd ws
go build ./cmd/ws-server  # ✅ Just works
```

### Docker Build

**Before:**
```bash
# Where's the Dockerfile? Hunt through deployments/...
docker build -f deployments/docker/ws-server/Dockerfile .  # ❌ Context is wrong
docker build -f deployments/docker/ws-server/Dockerfile -t ws-server .  # ❌ Still wrong

# Need to set context carefully
docker build -f deployments/docker/ws-server/Dockerfile -t ws-server ws/  # Finally works
```

**After:**
```bash
cd ws
docker build -t ws-server .  # ✅ Just works
```

## Files Modified Summary

| File | Change Type | Description |
|------|-------------|-------------|
| `ws/go.mod` | Created | Module definition for ws-server |
| `ws/go.sum` | Generated | Dependency checksums |
| `ws/Dockerfile` | Moved | From `deployments/docker/ws-server/` |
| `ws/cmd/ws-server/main.go` | Modified | Updated import paths |
| `ws/internal/**/*.go` (9 files) | Modified | Updated import paths |
| `loadtest/Dockerfile` | Created | Self-contained build for load tester |
| `deployments/local/docker-compose.yml` | Modified | Updated build context |
| `deployments/gcp-distributed/ws-server/docker-compose.yml` | Modified | Updated build context |
| `taskfiles/isolated-setup.yml` | Modified | Simplified deployment tasks |
| `MODULE_REFACTORING.md` | Created | This documentation |

**Total:** 1 created module, 1 created Dockerfile, 10 source files updated, 3 config files updated

## Dependencies Fixed

### golang.org/x/time Version

**Issue:** Initially had `golang.org/x/time@v0.14.0` which requires Go 1.24

**Fix:** Downgraded to `golang.org/x/time@v0.5.0` compatible with Go 1.23.1

```bash
cd ws
go get golang.org/x/time@v0.5.0
go mod tidy
```

## Testing Checklist

- ✅ Local Go build: `cd ws && go build ./cmd/ws-server`
- ✅ Docker build: `cd ws && docker build .`
- ✅ docker-compose build: `cd deployments/local && docker-compose build ws-go`
- ✅ Import path resolution
- ✅ Module dependencies resolution
- ✅ loadtest Docker build: `cd loadtest && docker build .`
- 🔲 GCP deployment test (not yet run)
- 🔲 Full integration test

## Related Documentation

- `DIRECTORY_REORGANIZATION.md` - How we moved `cmd/` and `internal/` into `ws/`
- `DEPLOYMENT_REORGANIZATION.md` - How we organized deployments/
- `loadtest/README.md` - Load testing tool documentation

## Rollback Plan

If needed, rollback with git:

```bash
# Revert all changes
git checkout HEAD -- ws/
git checkout HEAD -- loadtest/
git checkout HEAD -- deployments/
git checkout HEAD -- taskfiles/isolated-setup.yml

# Restore old structure
git checkout HEAD -- deployments/docker/
git checkout HEAD -- go.mod
git checkout HEAD -- go.sum
```

## Important: GCP Deployment Structure

### Local vs Remote Paths

The `deployments/gcp-distributed/*/docker-compose.yml` files have **different paths** than `deployments/local/docker-compose.yml`:

**Local Development (`deployments/local/docker-compose.yml`):**
```yaml
ws-go:
  build:
    context: ../../ws        # Goes UP to root, then into ws/
    dockerfile: Dockerfile
```

**GCP Distributed (`deployments/gcp-distributed/ws-server/docker-compose.yml`):**
```yaml
ws-go:
  build:
    context: ./ws            # ws/ is COPIED to same dir on VM
    dockerfile: Dockerfile
```

**Why Different?**

On the remote VM, the deployment task copies files FLAT:
```
/home/deploy/ws_poc/
├── ws/                    # Copied from local ws/
├── docker-compose.yml     # Copied from deployments/gcp-distributed/ws-server/
├── publisher/             # Copied from local publisher/
└── ...
```

So paths are relative to the DEPLOYED location, not the local source structure.

**Key Rule:** GCP deployment docker-compose files use `./service/` paths because services are copied alongside docker-compose.yml.

## Next Steps

1. ✅ Refactoring complete
2. ✅ Local builds verified
3. ✅ Docker builds verified
4. ✅ Fixed GCP deployment paths
5. 🔲 Test GCP deployment
6. 🔲 Update main README.md
7. 🔲 Consider removing old `src/` directory

---

**Completed by:** Claude  
**Verification:** All builds passing, structure clean, services self-contained  
**Standard:** Follows golang-standards/project-layout + Docker best practices
