# Local Development Guide

Guide for running and developing the WebSocket server locally without Docker.

## Prerequisites

- **Go 1.25.1+** - [Install Go](https://go.dev/doc/install)
- **Node.js 22+** - [Install Node.js](https://nodejs.org/)
- **Task** - [Install Task](https://taskfile.dev/installation/)
- **Docker** (for NATS only) - [Install Docker](https://docs.docker.com/get-docker/)

## Quick Start

### 1. Start NATS Container

```bash
task dev:nats
```

This starts only the NATS container on `localhost:4222`.

### 2. Run Go Server Locally

```bash
# Terminal 1
task dev:go
```

Server will start on `http://localhost:3002`

### 3. Run Publisher Locally (Optional)

```bash
# Terminal 2
task dev:publisher
```

Publisher API will start on `http://localhost:3003`

## Project Structure

```
/src/                   # Go WebSocket server
  ├── *.go             # Go source files
  ├── go.mod           # Go dependencies
  ├── Dockerfile        # Production Docker image
  └── docs/            # Architecture documentation

/publisher/            # Node.js NATS publisher
  ├── publisher.ts     # Main publisher file
  ├── config/          # Configuration
  ├── types/           # TypeScript types
  ├── package.json     # Node dependencies
  ├── tsconfig.json    # TypeScript config
  └── Dockerfile       # Production Docker image

/scripts/              # Testing scripts
  ├── stress-test-high-load.cjs
  ├── test-connection-rate.cjs
  └── ...

/docs/                 # Documentation
  ├── architecture/    # System architecture docs
  ├── monitoring/      # Monitoring guides
  └── development/     # Development guides
```

## Development Workflow

### Go Server Development

1. **Install Dependencies**
   ```bash
   cd src
   go mod download
   ```

2. **Build Locally**
   ```bash
   go build -o server .
   ```

3. **Run Server**
   ```bash
   ./server -addr=:3002 -nats=nats://localhost:4222
   ```

4. **Or use Task**
   ```bash
   task dev:go
   ```

5. **Format Code**
   ```bash
   gofmt -w .
   # Or
   task format:go
   ```

### Publisher Development

1. **Install Dependencies**
   ```bash
   cd publisher
   npm install
   ```

2. **Run in Development Mode**
   ```bash
   npm run dev
   ```

3. **Build TypeScript**
   ```bash
   npm run build
   ```

4. **Run Built Version**
   ```bash
   npm start
   ```

5. **Or use Task**
   ```bash
   task dev:publisher
   ```

6. **Format Code**
   ```bash
   npm run format
   # Or
   task format:ts
   ```

## Testing Locally

### Run Stress Tests

```bash
# Light load
task test:light

# Medium load
task test:medium

# Heavy load
task test:heavy

# Custom load
node scripts/stress-test-high-load.cjs 100 30 go
```

The test script will connect to `ws://localhost:3004/ws` by default.

### Manual Testing

**Connect with wscat:**
```bash
npm install -g wscat
wscat -c ws://localhost:3002/ws
```

**Send test message:**
```json
{"type":"ping","timestamp":1234567890}
```

### Check Health

```bash
curl http://localhost:3002/health | jq '.'
```

### View Metrics

```bash
curl http://localhost:3002/metrics
```

## Debugging

### Go Server Debugging (VSCode)

Create `.vscode/launch.json`:

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Launch Go Server",
      "type": "go",
      "request": "launch",
      "mode": "debug",
      "program": "${workspaceFolder}/src",
      "args": [
        "-addr=:3002",
        "-nats=nats://localhost:4222"
      ]
    }
  ]
}
```

### Publisher Debugging (VSCode)

Create `.vscode/launch.json`:

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Launch Publisher",
      "type": "node",
      "request": "launch",
      "runtimeExecutable": "tsx",
      "runtimeArgs": ["publisher.ts"],
      "cwd": "${workspaceFolder}/publisher",
      "env": {
        "NATS_URL": "nats://localhost:4222",
        "PORT": "3003"
      }
    }
  ]
}
```

### Verbose Logging

**Go Server:**
```bash
# Add log statements
log.Printf("Debug: %+v", variable)
```

**Publisher:**
```bash
# Console logging is already enabled
console.log('Debug:', data)
```

## Hot Reloading

### Go Server (Air)

Install Air:
```bash
go install github.com/air-verse/air@latest
```

Create `.air.toml` in `/src`:
```toml
root = "."
testdata_dir = "testdata"
tmp_dir = "tmp"

[build]
  args_bin = ["-addr=:3002", "-nats=nats://localhost:4222"]
  bin = "./tmp/main"
  cmd = "go build -o ./tmp/main ."
  delay = 1000
  exclude_dir = ["assets", "tmp", "vendor", "testdata"]
  exclude_file = []
  exclude_regex = ["_test.go"]
  exclude_unchanged = false
  follow_symlink = false
  full_bin = ""
  include_dir = []
  include_ext = ["go", "tpl", "tmpl", "html"]
  include_file = []
  kill_delay = "0s"
  log = "build-errors.log"
  poll = false
  poll_interval = 0
  post_cmd = []
  pre_cmd = []
  rerun = false
  rerun_delay = 500
  send_interrupt = false
  stop_on_error = false

[color]
  app = ""
  build = "yellow"
  main = "magenta"
  runner = "green"
  watcher = "cyan"

[log]
  main_only = false
  time = false

[misc]
  clean_on_exit = false

[screen]
  clear_on_rebuild = false
  keep_scroll = true
```

Run with hot reload:
```bash
cd src
air
```

### Publisher (Nodemon)

Install nodemon:
```bash
npm install -g nodemon
```

Run with hot reload:
```bash
cd publisher
nodemon --exec tsx publisher.ts
```

## Common Issues

### Port Already in Use

**Error**: `bind: address already in use`

**Solution:**
```bash
# Find process using port
lsof -ti:3002

# Kill process
kill -9 $(lsof -ti:3002)
```

### NATS Connection Failed

**Error**: `NATS connection failed`

**Solutions:**
1. Ensure NATS is running:
   ```bash
   task dev:nats
   ```

2. Check NATS is accessible:
   ```bash
   curl http://localhost:8222/healthz
   ```

### Go Module Issues

**Error**: `cannot find package`

**Solution:**
```bash
cd src
go mod tidy
go mod download
```

### TypeScript Build Errors

**Error**: TypeScript compilation failures

**Solution:**
```bash
cd publisher
npm install
npm run build
```

## Environment Variables

### Go Server

Set via command-line flags:
- `-addr` - Server address (default: `:3002`)
- `-nats` - NATS URL (default: `nats://localhost:4222`)

### Publisher

Set in `.env` or environment:
- `NATS_URL` - NATS connection URL
- `PORT` - HTTP API port (default: `3003`)
- `NODE_ENV` - Environment (development/production)
- `TOKENS` - Comma-separated token list

Example `.env`:
```env
NATS_URL=nats://localhost:4222
PORT=3003
NODE_ENV=development
TOKENS=BTC,ETH,ODIN,SOL,DOGE
```

## Performance Profiling

### Go Server Profiling

Add pprof endpoint in `main.go`:
```go
import _ "net/http/pprof"

// In main()
go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()
```

Access profiles:
```bash
# CPU profile
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# Memory profile
go tool pprof http://localhost:6060/debug/pprof/heap

# Goroutines
go tool pprof http://localhost:6060/debug/pprof/goroutine
```

### Publisher Profiling

Use Node.js built-in profiler:
```bash
node --prof dist/publisher.js
node --prof-process isolate-*.log > profile.txt
```

## Tips

1. **Use Task Commands**: Prefer `task` commands over manual commands for consistency

2. **Keep NATS Running**: Leave NATS container running during development

3. **Separate Terminals**: Use separate terminal windows/panes for each service

4. **Check Logs**: Always check logs when debugging issues

5. **Clean Builds**: Run `task clean` before building to avoid stale artifacts

6. **Test Frequently**: Run stress tests frequently during development

7. **Monitor Resources**: Keep an eye on CPU and memory usage during development
