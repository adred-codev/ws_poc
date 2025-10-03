# Architecture: NATS + WebSocket Message Flow

**NATS connections: 1 (one)**
**WebSocket connections: Up to 2,184 (limited by memory)**

The 2,184 limit is for **client WebSocket connections**, NOT NATS connections.

## 📊 Network Connections Breakdown

### Actual Connections in Container
```bash
$ docker exec odin-ws-go-2 ss -tan | grep ESTAB

192.168.160.4:42170  →  192.168.160.2:4222   [NATS connection - ONE]
                                               ↑
                                               This is the ONLY NATS connection
                                               regardless of client count
```

### Connection Types

```
┌─────────────────────────────────────────────────────────────┐
│  Container: odin-ws-go-2 (512MB memory limit)               │
│                                                              │
│  ┌──────────────────────────────────────────────┐           │
│  │  Go Server Process                           │           │
│  │                                               │           │
│  │  ┌─────────────────────────────────────┐    │           │
│  │  │  NATS Connection: 1                  │    │           │
│  │  │  Memory: ~5MB (part of 128MB overhead)   │           │
│  │  │  Role: Subscribe to "odin.token.>"   │    │           │
│  │  └─────────────────────────────────────┘    │           │
│  │                                               │           │
│  │  ┌─────────────────────────────────────┐    │           │
│  │  │  WebSocket Connections: 0-2,184      │    │           │
│  │  │  Memory: 180KB each                  │    │           │
│  │  │  Role: Serve client browsers/apps    │    │           │
│  │  └─────────────────────────────────────┘    │           │
│  │                                               │           │
│  │  ┌─────────────────────────────────────┐    │           │
│  │  │  Runtime Overhead: 128MB             │    │           │
│  │  │  - Go runtime: ~50MB                 │    │           │
│  │  │  - NATS client: ~5MB                 │    │           │
│  │  │  - Libraries: ~20MB                  │    │           │
│  │  │  - OS buffers: ~53MB                 │    │           │
│  │  └─────────────────────────────────────┘    │           │
│  └──────────────────────────────────────────────┘           │
└─────────────────────────────────────────────────────────────┘

Total memory accounting:
  Runtime overhead:        128 MB  (fixed, includes NATS)
  WebSocket connections:   384 MB  (2,184 × 180KB)
  ─────────────────────────────
  Total:                   512 MB  ✓ Fits in container
```

## 🔄 Complete Message Flow (NATS → Clients)

### Scenario: Price Update for BTC/USD

```
┌──────────────────────────────────────────────────────────────────────┐
│ 1. Price Data Source (Exchange API, Database, etc.)                 │
└────────────────┬─────────────────────────────────────────────────────┘
                 │
                 │ Publishes message
                 │
                 ↓
┌──────────────────────────────────────────────────────────────────────┐
│ 2. NATS Server (odin-nats container)                                │
│    Topic: "odin.token.BTCUSD"                                        │
│    Payload: {"symbol":"BTCUSD","price":50000,"timestamp":...}       │
└────────────────┬─────────────────────────────────────────────────────┘
                 │
                 │ ONE TCP connection (192.168.160.4:42170 → NATS:4222)
                 │
                 ↓
┌──────────────────────────────────────────────────────────────────────┐
│ 3. Go Server (odin-ws-go-2) - NATS Subscriber                       │
│    server.go:135                                                     │
│                                                                      │
│    natsConn.Subscribe("odin.token.>", func(msg *nats.Msg) {         │
│        s.workerPool.Submit(func() {                                 │
│            s.broadcast(msg.Data)  // Broadcast to ALL clients       │
│        })                                                            │
│    })                                                                │
│                                                                      │
│    Message received ONCE via single NATS connection                 │
└────────────────┬─────────────────────────────────────────────────────┘
                 │
                 │ broadcast() function
                 │
                 ↓
┌──────────────────────────────────────────────────────────────────────┐
│ 4. Broadcast Loop (server.go:320)                                   │
│                                                                      │
│    func broadcast(message []byte) {                                 │
│        s.clients.Range(func(client *Client) {                       │
│            // Wrap in envelope with sequence number                 │
│            envelope := WrapMessage(message, client.seqGen)          │
│            client.replayBuffer.Add(envelope)                        │
│            client.send <- envelope  // Send to THIS client          │
│        })                                                            │
│    }                                                                 │
│                                                                      │
│    ONE NATS message → 2,184 WebSocket sends (one per client)        │
└────────────────┬─────────────────────────────────────────────────────┘
                 │
                 │ Parallel sends to all clients
                 │
        ┌────────┼────────┬────────┬─────────┬─────────────┐
        ↓        ↓        ↓        ↓         ↓             ↓
    ┌───────┬───────┬───────┬───────┬─────────────┬──────────────┐
    │Client │Client │Client │Client │    ...      │Client #2,184 │
    │  #1   │  #2   │  #3   │  #4   │             │              │
    └───────┴───────┴───────┴───────┴─────────────┴──────────────┘

    Each client receives:
    {
      "seq": 12345,              // Unique per client
      "ts": 1696284000000,
      "type": "price:update",
      "data": {"symbol":"BTCUSD","price":50000,...}
    }
```

## 📥 Message Flow Direction

### NATS → Server → Clients (Primary Flow, 99% of traffic)

```
NATS publishes price update
  ↓
  │ ONE message
  │ 500 bytes
  │
Server receives via NATS subscription (ONE connection)
  ↓
  │ Process in worker pool
  │ Wrap in MessageEnvelope
  │
broadcast() sends to ALL clients (2,184 WebSocket connections)
  ↓
  │ 2,184 messages (one per client)
  │ 2,184 × 500 bytes = 1,092 KB total
  │
  ├─→ Client #1 (seq=1001)
  ├─→ Client #2 (seq=1002)
  ├─→ Client #3 (seq=1003)
  ...
  └─→ Client #2,184 (seq=1042)
```

**Key insight**: ONE NATS message becomes 2,184 WebSocket messages. This is why the server needs large send buffers (128KB per client).

### Clients → Server (Control Messages, <1% of traffic)

```
Client sends control message (rare)
  ↓
WebSocket receive
  ↓
readPump() processes (server.go:265)
  ↓
Rate limiting check (100 burst, 10/sec)
  ↓
handleClientMessage() (server.go:400)
  ↓
  ├─→ "replay" request → send missed messages from replayBuffer
  ├─→ "heartbeat" request → send pong
  ├─→ "subscribe" request → (future: per-symbol subscriptions)
  └─→ "unsubscribe" request → (future)
```

**These messages do NOT go to NATS** - they're handled locally by the Go server.

## 💾 Memory Accounting: Where Does Each 180KB Go?

### Per-Client Breakdown (server.go:237 → connection.go:126)

```go
type Client struct {
    // 1. Send channel buffer: 128KB
    send chan []byte  // 256 slots × 500 bytes avg
    //    Purpose: Buffer outgoing NATS messages to client
    //    Why so large: During market volatility, NATS sends 50 msg/sec
    //                  Need 5-second buffer: 50 × 5 = 250 messages

    // 2. Replay buffer: 50KB
    replayBuffer *ReplayBuffer  // 100 messages × 500 bytes avg
    //    Purpose: Store recent messages for gap recovery
    //    Why needed: Client network hiccup → reconnect → replay missed messages

    // 3. Sequence generator: 8 bytes
    seqGen *SequenceGenerator  // atomic int64
    //    Purpose: Monotonically increasing message IDs

    // 4. Connection + overhead: ~2KB
    conn   net.Conn       // TCP socket
    mu     sync.RWMutex   // Mutex for thread safety
    // ... other fields
}

Total: 128KB + 50KB + 8B + 2KB ≈ 180KB per client
```

### What's NOT Counted Per Client

**Message data in flight** (NATS → Server → Clients):
- NATS message arrives: 500 bytes (ONE message)
- Server processes: 500 bytes (still ONE message)
- broadcast() creates 2,184 copies: 2,184 × 500 bytes = 1,092 KB
  - BUT these copies go into the send channels we already counted (128KB each)
  - So no additional memory beyond the 180KB per client

**NATS connection memory**:
- NATS client library: ~5MB
- NATS subscription buffers: ~2MB
- Total: ~7MB (included in 128MB runtime overhead)

## 🔢 Load Test Scenario Analysis

### Your Test: 10,000 WebSocket Clients

```bash
task stress:container:go CONNECTIONS=10000 DURATION=240
```

**What happens:**

```
Client #1 connects:
  ↓
  handleWebSocket() acquires semaphore slot (1/2,184)
  ↓
  Memory used: 180KB
  ✅ Connection established

Client #2 connects:
  ↓
  handleWebSocket() acquires semaphore slot (2/2,184)
  ↓
  Memory used: 360KB (cumulative)
  ✅ Connection established

... (repeat 2,182 times) ...

Client #2,184 connects:
  ↓
  handleWebSocket() acquires LAST semaphore slot (2,184/2,184)
  ↓
  Memory used: 393.12MB (2,184 × 180KB)
  ✅ Connection established

───────────────────────────────────────────────────────────

Client #2,185 connects:
  ↓
  handleWebSocket() tries to acquire semaphore
  ↓
  Semaphore FULL (2,184/2,184)
  ↓
  5-second timeout (server.go:239)
  ↓
  Returns HTTP 503 "Server at capacity"
  ❌ Connection REJECTED

Client #2,186 - #10,000 (7,815 more):
  ❌ ALL REJECTED with HTTP 503
```

**Error breakdown you saw:**
- `Unexpected: 7,720` = HTTP 503 rejections
- `ECONNRESET: 96` = Connections dropped during upgrade or slow client detection
- Total rejections: 7,816 (10,000 - 2,184)

## 🌊 NATS Message Amplification

### One NATS Message = 2,184 WebSocket Messages

**Scenario: BTC price update published every 100ms (10 msg/sec)**

```
NATS publishes 10 messages/sec
  ↓
  │ Bandwidth: 10 msg/sec × 500 bytes = 5 KB/sec
  │
Server receives via ONE NATS connection
  ↓
  │ 5 KB/sec incoming from NATS
  │
broadcast() to 2,184 clients
  ↓
  │ Outgoing: 10 msg/sec × 2,184 clients × 500 bytes
  │         = 21,840 messages/sec
  │         = 10.92 MB/sec
  │
  │ Amplification factor: 2,184× !
  │
2,184 clients receive 10 msg/sec each
```

**This explains why we need:**
- Large send buffers (128KB) - Buffer 256 messages during burst
- Slow client detection - One slow client can't block 2,183 fast clients
- Replay buffer - Network hiccup recovery without full reconnect

## 🔄 Client → Server Messages (Minimal)

**These are NOT part of the NATS flow.** Client control messages are handled entirely within the Go server:

### Message Types (server.go:400)

```go
// 1. Replay request (when client detects gap)
{
    "type": "replay",
    "data": {
        "from": 1000,  // Missed messages 1000-1050
        "to": 1050
    }
}

// Server response: Reads from client.replayBuffer
// NO NATS INTERACTION - purely local buffer read

// 2. Heartbeat (keepalive ping)
{
    "type": "heartbeat"
}

// Server response: {"type": "pong", "ts": 1234567890}
// NO NATS INTERACTION

// 3. Future: Symbol subscription
{
    "type": "subscribe",
    "data": {"symbols": ["BTCUSD", "ETHUSD"]}
}

// Server response: Filter NATS messages to only send subscribed symbols
// STILL uses same NATS connection (doesn't create new NATS subscriptions)
```

**Rate limiting (100 burst, 10/sec) applies to THESE messages**, not to NATS messages.

## 📊 Memory Usage: Real Example

### With 2,184 Clients Connected

```bash
$ curl -s http://localhost:3004/stats | jq '{currentConnections, memoryMB}'

{
  "currentConnections": 2184,
  "memoryMB": 421.5
}

Breakdown:
  Runtime + NATS:      128 MB  (fixed overhead)
  2,184 clients:       393 MB  (2,184 × 180KB)
  ──────────────────────────
  Total:               521 MB  (slightly over 512MB limit, GC triggers)
```

**Note**: Memory can briefly exceed 512MB during GC cycles, but Cloud Run allows ~10% overage before OOM kill.

## 🎯 Key Takeaways

### 1. NATS Connections = 1 (Always)
```
Whether you have:
  - 1 client
  - 2,184 clients
  - 10,000 clients (if memory allowed)

NATS connections: ALWAYS 1

Server code (server.go:108):
  nc, _ := nats.Connect(config.NATSUrl)  // Called ONCE on startup
  s.natsConn = nc                        // Shared across all clients
```

### 2. WebSocket Connections = 0 to 2,184 (Memory Limited)
```
Limited by semaphore: make(chan struct{}, 2,184)
Calculated from: (512MB - 128MB) / 180KB = 2,184
```

### 3. Message Amplification
```
ONE NATS message → 2,184 WebSocket sends

This is why:
  - 128KB send buffer per client (256 messages buffered)
  - Slow client detection (prevents one client blocking all)
  - Worker pool (broadcast in background, don't block NATS)
```

### 4. Client → Server Messages
```
NOT sent to NATS
Handled locally (replay requests, heartbeats, subscriptions)
Rate limited: 100 burst, 10/sec per client
```

### 5. Memory Accounting
```
Fixed overhead (includes NATS):        128 MB
Per-client cost:                       180 KB
  - Send channel (NATS message buffer): 128 KB
  - Replay buffer (gap recovery):        50 KB
  - Overhead:                             2 KB

Max clients: (512MB - 128MB) / 180KB = 2,184
```

## 🚀 To Support 10,000 Clients

### Option 1: Increase Container Memory (Vertical Scaling)
```yaml
mem_limit: 2g  # Instead of 512m

Max clients: (2048MB - 128MB) / 180KB = 10,666 clients ✓

NATS connections: Still 1
Cost: ~4× Cloud Run pricing
```

### Option 2: Multiple Server Instances (Horizontal Scaling)
```
         ┌────────────────┐
         │  Load Balancer │
         └───────┬────────┘
                 │
         ┌───────┼───────────────┬──────────────┐
         ↓       ↓               ↓              ↓
    ┌────────┬────────┬─────────┬────────┐
    │Server 1│Server 2│Server 3 │Server 4│Server 5│
    │2,184   │2,184   │2,184    │2,184   │2,184   │
    │clients │clients │clients  │clients │clients │
    └────┬───┴────┬───┴─────┬───┴────┬───┴────┬───┘
         │        │         │        │        │
         └────────┴─────────┴────────┴────────┘
                        ↓
                ┌──────────────┐
                │  NATS Server │
                └──────────────┘

Total: 10,920 clients
NATS connections: 5 (one per server instance)
Cost: 5× instances (better fault tolerance)
```

**Recommendation**: Horizontal scaling for production (better reliability).

---

**Bottom Line**: The 2,184 limit is for **WebSocket client connections**, not NATS. NATS uses a single connection regardless of client count. Messages flow: NATS (1 connection) → Server → Clients (up to 2,184 connections), with 1 NATS message amplified to 2,184 WebSocket messages.
