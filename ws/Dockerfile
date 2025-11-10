# Stage 1: Build the application
FROM golang:1.25.1-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum files to download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the entire source tree (flat structure - all .go files at root)
COPY . .

# Build the application from current directory (main.go is at root)
# -ldflags="-s -w" strips debug information, creating a smaller binary
# CGO_ENABLED=0 ensures a static binary without C dependencies
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /odin-ws-server .

# Stage 2: Create the final, minimal image
FROM alpine:latest

WORKDIR /root/

# Install ca-certificates for HTTPS connections (Kafka/Redpanda TLS support)
RUN apk --no-cache add ca-certificates

# Copy the static binary from the builder stage
COPY --from=builder /odin-ws-server .

# Expose the port the server will listen on
EXPOSE 3002

# Command to run the application
# Configuration is loaded from environment variables (see .env.production)
CMD ["./odin-ws-server"]
