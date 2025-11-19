# Sharded WebSocket Server Implementation Plan

## 1. Overview

The goal of this plan is to implement a sharded, multi-core WebSocket server. The current single-core implementation becomes CPU-bound under heavy load, preventing it from reaching the target of 12,000 concurrent clients.

This new architecture will distribute the workload across multiple CPU cores, significantly improving performance, scalability, and allowing us to meet and exceed the target client load.

## 2. Architecture: Sharded Workers

The new design is a multi-process "shard" architecture, where each shard is an independent worker running on a dedicated CPU core. A manager process will be responsible for creating, managing, and load-balancing these shards.

```
                               +-----------------+
                               |  Load Balancer  |
                               | (Listens on Port)|
                               +-----------------+
                                       |
                  (Distributes new connections via "Least Connections")
                                       |
      +--------------------------------+--------------------------------+
      |                                |                                |
+-----v-----+                    +-----v-----+                    +-----v-----+
|  Shard 1  |                    |  Shard 2  |                    |  Shard 3  |
|  (Core 1) |                    |  (Core 2)  |                    |  (Core 3)  |
+-----------+                    +-----------+                    +-----------+
| - Kafka Consumer               | - Kafka Consumer               | - Kafka Consumer               |
| - Connection Pool (4k clients) | - Connection Pool (4k clients) | - Connection Pool (4k clients) |
| - Subscription Index           | - Subscription Index           | - Subscription Index           |
| - Write Pump                   | - Write Pump                   | - Write Pump                   |
+--------------------------------+--------------------------------+--------------------------------+
      ^                                ^                                ^
      | (Subscribes to Bus)            | (Subscribes to Bus)            | (Subscribes to Bus)
      |                                |                                |
      +--------------------------------+--------------------------------+
                                       |
                               +-----------------+
                               |  Broadcast Bus  |
                               | (Go Channels)   |
                               +-----------------+
                                       ^
                                       |
                                (Publishes to Bus)
```

## 3. Implementation Phases

### Phase 1: Refactor for Shared Components (Completed)

This phase is complete. The goal was to move all reusable code into the `ws/internal/shared` directory to serve as a foundation for both the `single` and `multi` core versions.

- **Status:** All core logic, types, and utility packages have been moved to `ws/internal/shared`. The `single` core version has been updated to use these shared components and compiles successfully.

### Phase 2: Multi-Core Implementation

This is the current phase, where we will build the multi-core orchestration layer.

1.  **Implement the `Shard` (`ws/internal/multi/shard.go`):**
    *   Define a `Shard` struct that encapsulates an instance of the `shared.Server`.
    *   Each shard will have its own Kafka consumer with a unique consumer group ID (e.g., `my-group-1`, `my-group-2`).
    *   The shard's Kafka consumer will be configured to publish messages to the `BroadcastBus` instead of broadcasting them directly.

2.  **Implement the `BroadcastBus` (`ws/internal/multi/broadcast.go`):**
    *   Create a central, in-memory pub/sub system using Go channels.
    *   It will have a `Publish` method for shards to send messages to the bus.
    *   It will have a `Subscribe` method that provides a channel for each shard to listen for all broadcast messages.
    *   A central goroutine will manage the fan-out of messages from the publish channel to all subscriber channels.

3.  **Implement the `LoadBalancer` (`ws/internal/multi/loadbalancer.go`):**
    *   Create a `LoadBalancer` that listens on the main public-facing port.
    *   Implement the **"Least Connections"** strategy:
        *   Each shard will expose its current number of active connections.
        *   The load balancer will query all shards and forward new connections to the shard with the fewest active connections.
    *   Implement **Capacity-Awareness**:
        *   The load balancer will also query each shard's maximum capacity (`WS_MAX_CONNECTIONS / num_shards`).
        *   If a shard is at capacity, it will be excluded from the selection.
        *   If all shards are at capacity, the load balancer will reject the new connection with an HTTP `503 Service Unavailable` status.
    *   The load balancer will use `httputil.ReverseProxy` to forward the WebSocket connection to the selected shard's internal listener.

4.  **Create the `main` Entrypoint (`ws/cmd/multi/main.go`):**
    *   This will be the main orchestrator.
    *   It will parse command-line flags for the number of shards (`-shards`) and the load balancer address (`-lb-addr`).
    *   It will calculate the per-shard connection limit based on `WS_MAX_CONNECTIONS`.
    *   It will initialize the `BroadcastBus`, create and run the `Shard` instances, and start the `LoadBalancer`.
    *   It will handle graceful shutdown signals to ensure all components (shards, bus, balancer) shut down cleanly.

### Phase 3: Configuration and Testing

1.  **Create `docker-compose.multi.yml`:**
    *   A new Docker Compose file will be created to build and run the `multi` core executable.
    *   It will be configured with appropriate resource limits for the `e2-standard-4` instance (e.g., 3 CPUs, 14GB RAM).

2.  **Update `Taskfile.yml`:**
    *   New tasks will be added for the multi-core version (e.g., `task local:multi:up`, `task local:multi:test`).

3.  **Run 12k Capacity Test:**
    *   Execute the 12,000-connection load test against the new multi-core server.
    *   Monitor CPU and memory usage in Grafana.
    *   Analyze the load test logs to verify that all connections are successful and that messages are being received correctly.
    *   Confirm that the CPU bottleneck is resolved and that the load is distributed across multiple cores.

## 4. Edge Cases and Considerations

*   **Graceful Shutdown:** The shutdown sequence will be carefully managed: stop the load balancer first (no new connections), then shut down the shards, and finally the broadcast bus.
*   **Connection Distribution:** The "Least Connections" strategy will prevent shard imbalance.
*   **Inter-Shard Communication:** The `BroadcastBus` using buffered Go channels is highly efficient for a single-machine setup.
*   **Configuration:** The number of shards will be configurable via a command-line flag, allowing for easy tuning.
*   **Resource Pinning:** For a future optimization, we could explore pinning each shard's goroutines to a specific OS thread (and thus a specific CPU core) using `runtime.LockOSThread`. For this initial implementation, we will rely on the Go runtime's scheduler.
