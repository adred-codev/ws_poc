# Stage 1: Build the application
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum files to download dependencies
COPY go.mod go.sum ./
RUN go mod tidy
RUN go mod download

# Copy the source code
COPY . .

# Build the application
# -ldflags="-s -w" strips debug information, creating a smaller binary
# CGO_ENABLED=0 ensures a static binary without C dependencies
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /odin-ws-server-2 .

# Stage 2: Create the final, minimal image
FROM alpine:latest

WORKDIR /root/

# Copy the static binary from the builder stage
COPY --from=builder /odin-ws-server-2 .

# Expose the port the server will listen on
EXPOSE 3002

# Command to run the application
# The server will be started with the address and NATS URL passed as arguments
# e.g., docker run -p 3002:3002 my-image -addr=":3002" -nats="nats://nats:4222"
CMD ["./odin-ws-server-2"]
