# syntax=docker/dockerfile:1

# Stage 1: build static binary
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source
COPY . ./

# Build odin-ws-server-3 binary
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /odin-ws-server-3 ./cmd/odin-ws

# Stage 2: minimal runtime image
FROM alpine:3.20

RUN addgroup -S odin && adduser -S -G odin odin
WORKDIR /srv

# Copy compiled binary
COPY --from=builder /odin-ws-server-3 ./odin-ws-server-3

# Expose WebSocket & metrics ports
EXPOSE 8082 9095

USER odin
ENTRYPOINT ["./odin-ws-server-3"]
