# Test Runner Go - Isolated Setup
# Go-based load test client - solves Node.js single-threaded event loop bottleneck
#
# Key difference from Node.js test runner:
# - Node.js: Single event loop, 1 CPU core saturated at 7K connections (bottleneck)
# - Go: 7,000 independent goroutines, distributed across all CPU cores (no bottleneck)
#
# See: docs/NODE_JS_EVENT_LOOP_BOTTLENECK.md

# Stage 1: Build the application
FROM golang:1.23-alpine AS builder

WORKDIR /build

# Copy go.mod and go.sum files to download dependencies
COPY go.mod go.sum* ./
RUN go mod tidy
RUN go mod download

# Copy the source code
COPY . .

# Build the application
# -ldflags="-s -w" strips debug information, creating a smaller binary
# CGO_ENABLED=0 ensures a static binary without C dependencies
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /sustained-load-test .

# Stage 2: Create the final, minimal image
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the static binary from the builder stage
COPY --from=builder /sustained-load-test .

# Default command - runs Go load test
# Override environment variables as needed:
# - WS_URL: WebSocket server URL (default: ws://localhost:3004/ws)
# - TARGET_CONNECTIONS: Number of connections to create (default: 7000)
# - RAMP_RATE: Connections per second during ramp-up (default: 100)
# - DURATION: Sustain duration in seconds (default: 1800)
CMD ["./sustained-load-test"]
