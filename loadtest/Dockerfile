# Load Test Tool - WebSocket Load Tester
# Go-based load testing tool using goroutines for high concurrency
# No event loop bottleneck - can handle 10K+ concurrent connections

# Stage 1: Build the application
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Copy go.mod and go.sum files to download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the application (main.go)
# -ldflags="-s -w" strips debug information, creating a smaller binary
# CGO_ENABLED=0 ensures a static binary without C dependencies
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /loadtest .

# Stage 2: Create the final, minimal image
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the static binary from the builder stage
COPY --from=builder /loadtest .

# Default command - runs load test
# Override environment variables as needed:
# - WS_URL: WebSocket server URL (default: ws://localhost:3004/ws)
# - HEALTH_URL: Health check endpoint (default: http://localhost:3004/health)
# - TARGET_CONNECTIONS: Number of connections to create (default: 7000)
# - RAMP_RATE: Connections per second during ramp-up (default: 100)
# - DURATION: Sustain duration in seconds (default: 1800)
# - SUBSCRIPTION_MODE: Subscription mode: all, single, random (default: random)
# - CHANNELS: Comma-separated list of channels (default: BTC.trade,ETH.trade,SOL.trade,ODIN.trade,DOGE.trade)
# - CHANNELS_PER_CLIENT: Channels per client for random mode (default: 3)
CMD ["./loadtest"]
