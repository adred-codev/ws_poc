# Horizontal Scaling Plan for the Sharded WebSocket Server

## 1. Overview

This document outlines the plan to evolve the sharded WebSocket server from a vertically-scaled, single-machine architecture to a horizontally-scaled, multi-machine cluster. The goal is to create a system that can handle massive numbers of concurrent connections (50,000+) by distributing the load across multiple `e2-standard-4` instances.

## 2. Prerequisites

This plan assumes the successful completion of the **Sharded WebSocket Server Implementation Plan**. The single-machine, multi-core architecture is the foundation upon which this horizontal scaling plan is built.

## 3. Architecture

The horizontally-scaled architecture introduces two key new components: an **External Load Balancer** and an **External Message Bus**.

```
+--------------------------------+
| Google Cloud Load Balancer     |
+--------------------------------+
               |
(Distributes connections to instances)
               |
+--------------------------------+--------------------------------+
|                                |                                |
|      +------------------+      |      +------------------+      |
|      | e2-standard-4 #1 |      |      | e2-standard-4 #2 |      |
|      +------------------+      |      +------------------+      |
|      | - Shard 1        |      |      | - Shard 4        |      |
|      | - Shard 2        |      |      | - Shard 5        |      |
|      | - Shard 3        |      |      | - Shard 6        |      |
|      +------------------+      |      +------------------+      |
|               ^                |                ^               |
|               | (Pub/Sub)      |                | (Pub/Sub)     |
|               v                |                v               |
+--------------------------------+--------------------------------+
               |                                |
               +--------------------------------+
                               |
                 +-----------------------------+
                 |   External Message Bus      |
                 |   (e.g., Redis or NATS)     |
                 +-----------------------------+
```

### Component Roles:

*   **External Load Balancer (e.g., Google Cloud Load Balancer):**
    *   The single entry point for all client connections.
    *   Distributes incoming WebSocket connections across the available server instances.
    *   Performs health checks on the instances and routes traffic only to healthy ones.

*   **WebSocket Server Instances:**
    *   Multiple `e2-standard-4` machines, each running the same multi-core `ws-server` application.
    *   Each instance runs its own set of shards (e.g., 3 shards per instance).
    *   Each shard's Kafka consumer publishes messages to the external message bus.
    *   Each shard subscribes to the external message bus to receive broadcast messages.

*   **External Message Bus (e.g., Redis Pub/Sub or NATS):**
    *   The new backbone for inter-shard communication, replacing the in-memory `BroadcastBus`.
    *   When a shard on any instance receives a message from Kafka, it publishes it to the bus.
    *   All shards on *all* instances are subscribed to the bus, so they all receive the message and can broadcast it to their local clients.

## 4. Implementation Steps

### Step 1: Set up the External Message Bus

1.  **Choose a Technology:**
    *   **Redis Pub/Sub:** Recommended for its simplicity, high performance, and low operational overhead. It's likely already available in your cloud environment.
    *   **NATS:** A more powerful, cloud-native option if you need features like message persistence or guaranteed delivery in the future.

2.  **Deploy and Configure:**
    *   Provision a managed Redis or NATS instance in your cloud environment (e.g., Google Cloud Memorystore for Redis).
    *   Ensure the instance is accessible from your WebSocket server instances.

### Step 2: Replace the `BroadcastBus`

1.  **Create a `BroadcastBus` Interface:**
    *   Define a Go interface for the `BroadcastBus` that includes the `Publish` and `Subscribe` methods.

2.  **Implement a `RedisBroadcastBus` (or `NatsBroadcastBus`):**
    *   Create a new struct that implements the `BroadcastBus` interface.
    *   This implementation will use a Redis or NATS client to connect to the external message bus.
    *   The `Publish` method will publish messages to a specific Redis/NATS channel.
    *   The `Subscribe` method will subscribe to that channel and pass the received messages to the shard.

3.  **Update the `main` Entrypoint:**
    *   Modify `ws/cmd/multi/main.go` to instantiate the `RedisBroadcastBus` instead of the in-memory one, based on a configuration flag or environment variable.

### Step 3: Configure the External Load Balancer

1.  **Provision a Load Balancer:**
    *   Create a Google Cloud Load Balancer (or equivalent in your cloud provider).
    *   Configure it as a global external TCP/SSL load balancer.

2.  **Configure Health Checks:**
    *   Create a health check that targets the `/health` endpoint of your WebSocket server instances.
    *   The load balancer will use this to determine which instances are healthy and can receive traffic.

3.  **Configure Backend Service and Frontend:**
    *   Create a backend service that includes all your WebSocket server instances.
    *   Create a frontend forwarding rule that directs incoming traffic on the public port (e.g., 443) to the backend service.

### Step 4: Deployment

1.  **Create `docker-compose.horizontal.yml`:**
    *   A new Docker Compose file for deploying the server instances.
    *   This will be similar to `docker-compose.multi.yml`, but will include the necessary environment variables to connect to the external message bus.

2.  **Update `Taskfile.yml`:**
    *   Add tasks for deploying and managing the horizontally-scaled cluster (e.g., `task gcp:deploy:horizontal`, `task gcp:scale:up`).

## 5. Testing

1.  **Run 12k Capacity Test:**
    *   Run the existing 12,000-connection load test against the new horizontally-scaled cluster to verify its correctness and performance.

2.  **Run Massive-Scale Load Test:**
    *   Create a new load test configuration for 50,000 or 100,000 connections.
    *   Run this test to verify the scalability of the architecture and identify any new bottlenecks.
    *   Monitor the CPU and memory usage of all server instances and the message bus.

## 6. Future Considerations

*   **Autoscaling:** Configure autoscaling for your instance group based on CPU utilization or the number of active connections. This will allow the cluster to automatically scale up and down to meet demand.
*   **Geo-distributed Deployments:** For even lower latency, you can deploy server instances in multiple geographic regions, all connecting to a globally-distributed message bus.
