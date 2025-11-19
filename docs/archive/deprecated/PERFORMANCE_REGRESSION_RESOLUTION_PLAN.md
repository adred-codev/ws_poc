# Performance Regression Resolution Plan

## 1. Assessment of the Situation

The analysis of the performance regression is excellent and entirely correct. The counter-intuitive result—where increasing CPU cores and shard count from 3 to 7 led to an 8.2% performance *decrease*—is a classic distributed systems problem.

The root cause has been accurately identified as **coordination overhead outweighing the benefits of parallelism**.

*   **Amdahl's Law in Action:** The performance of the system is limited by its "serial fraction"—the part of the work that cannot be parallelized. In this architecture, the coordination work grows linearly with the number of shards, and this has become the new bottleneck.
*   **The "Chatty" Broadcast Bus:** The primary source of this overhead is the in-memory `BroadcastBus`. For every message consumed from Kafka, the system performs a fan-out to all 7 shards, regardless of whether they have clients subscribed to that message's topic. This creates a significant amount of unnecessary work and context switching, which explains the 60% drop in per-core efficiency.

The conclusion is sound: for this specific architecture, fewer, "fatter" shards perform better than many "thin" shards.

## 2. Resolution Plan

The plan is twofold: first, we will immediately find the optimal configuration for the current architecture to meet our performance goals. Second, we will evolve the architecture to eliminate the underlying bottleneck, enabling true linear scaling in the future.

---

### Part 1: Immediate Optimization (Finding the "Sweet Spot")

**Hypothesis:** There is a peak performance point between 2 and 6 shards where the benefits of parallelism are maximized before the costs of coordination take over.

We will conduct a structured experiment to find this optimal shard-to-core ratio on the `e2-highcpu-8` instance.

**Proposed Test Matrix:**

| Shard Count | CPU Limit per Shard | Total CPU | Expected Connections/Shard |
| :---------- | :------------------- | :-------- | :------------------------- |
| **2 Shards**  | 3.5                  | 7         | 6,000                      |
| **3 Shards**  | 2.3                  | 7         | 4,000                      |
| **4 Shards**  | 1.75                 | 7         | 3,000                      |
| **5 Shards**  | 1.4                  | 7         | 2,400                      |

**Execution Steps:**

1.  **Configure:** For each test case in the matrix, update the `-shards` command-line flag for the `ws-server`.
2.  **Execute:** Run the standard 12,000-connection load test for each configuration.
3.  **Collect Data:** Record the total successful connections and the peak CPU/Memory usage from Grafana for each run.
4.  **Analyze:** Plot the results to visualize the performance curve and identify the configuration with the highest total connections. This will become our new optimized production configuration.

---

### Part 2: Long-Term Architectural Fix (Eliminating the Bottleneck)

The broadcast bus is the fundamental scaling limiter. To unlock true linear scaling, we must evolve it from a "broadcast" model to a **"targeted dispatch"** model.

**Concept: Smart Dispatch**

Instead of sending every message to every shard, we will only send a message to the *specific shard* that is managing the clients for that message's `tokenID`.

**Proposed Implementation: A Redis-backed Connection Registry**

1.  **Centralize Connection Mapping:**
    *   When a client connects to a shard and subscribes to a `tokenID`, the shard will write that mapping to a central, high-speed data store. **Redis is the ideal tool for this.**
    *   The mapping would be a simple key-value pair: `SET token-123 shard-5`.

2.  **Create an Intelligent `MessageDispatcher`:**
    *   The `BroadcastBus` component will be replaced with a new `MessageDispatcher`.
    *   When the `MessageDispatcher` receives a message from a shard's Kafka consumer, it will first perform a quick lookup in Redis: `GET token-123`.
    *   It will get back the answer: `shard-5`.
    *   It will then publish the message **only** to the specific channel for `shard-5`.

**Architectural Impact:**

```
+--------------------------------+
|         Message Source         |
|        (Kafka Consumer)        |
+--------------------------------+
               |
               v
+--------------------------------+
|      Message Dispatcher        |
+--------------------------------+
| 1. Receives message for TokenX |
| 2. Asks Redis: "Who has TokenX?" --> "Shard 3"
| 3. Sends message ONLY to Shard 3
+--------------------------------+
       |                 ^
       | (Lookup)        | (Pub/Sub)
       v                 |
+--------------+   +----------------+
| Redis        |   | Shard Channels |
| (Token -> Shard)|   +----------------+
+--------------+
```

**Benefits of this approach:**

*   **Eliminates Fan-Out:** The N-to-N broadcast problem is completely solved. The coordination overhead is reduced to a constant-time O(1) Redis lookup.
*   **True Linear Scaling:** With this change, adding more shards has virtually zero additional coordination cost. Performance will increase linearly with the number of cores.
*   **Foundation for Horizontal Scaling:** This design is a prerequisite for the horizontal scaling plan, as it is inherently distributed.

## 3. Recommended Next Steps

1.  **Immediate Action:** Execute the test matrix from **Part 1** to find the optimal shard count for the current architecture and immediately resolve the performance regression.
2.  **Long-Term Strategy:** Plan a new development cycle to implement the **Targeted Dispatch System** from **Part 2**. This will be the key to unlocking the next level of performance and scalability for the platform.
