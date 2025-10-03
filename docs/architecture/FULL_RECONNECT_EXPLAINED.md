# Full Reconnect vs Replay: What's the Difference?

## 🎯 Your Question: "What does full reconnect mean?"

**Full reconnect** means closing the WebSocket connection completely and establishing a brand new connection, discarding all previous state.

## 📊 Full Reconnect vs Replay Comparison

### Option 1: Replay (Fast Recovery - What We Built)

```
Client has:
  - lastSeq = 1000
  - WebSocket still connected
  - Detected gap: 1001-1050 missing

Client action:
  ws.send({type: "replay", data: {from: 1001, to: 1050}})

Server response:
  Sends 50 messages from replay buffer

Result:
  ✅ Client receives missing messages
  ✅ Connection stays open
  ✅ Sequence continues: 1051, 1052, 1053...
  ✅ Total time: 20-50ms

Client state preserved:
  - Same WebSocket connection
  - Same sequence number tracking
  - Same server-side state (replay buffer intact)
```

### Option 2: Full Reconnect (Slow Recovery - Fallback)

```
Client has:
  - lastSeq = 1000
  - Gap too old to replay (beyond 100 message buffer)

Client action:
  1. ws.close()              // Close current connection
  2. localStorage.clear()    // Discard old state
  3. ws = new WebSocket()    // New connection
  4. Reset lastSeq = 0       // Start from scratch

Server sees:
  - Old connection closed
  - New connection established
  - New Client ID assigned
  - New replay buffer created
  - New sequence generator (starts at 0)

Result:
  ✅ Client gets NEW connection
  ✅ Receives messages starting from seq 1 (not 1001!)
  ✅ No historical messages (only new ones)
  ✅ Total time: 500-2000ms

Client state DISCARDED:
  - Different WebSocket connection
  - Sequence resets to 1
  - Server-side replay buffer is new/empty
```

## 🔄 Detailed Flow Comparison

### Scenario: Client Offline for 2 Minutes

**Setup:**
- Message rate: 10 msg/sec
- Client was at seq 1000 before disconnect
- Offline duration: 120 seconds
- Messages missed: 120s × 10 msg/sec = 1,200 messages
- Replay buffer size: 100 messages

---

### Flow 1: Replay Attempt (Fails - Gap Too Old)

```
┌─────────────────────────────────────────────────────────┐
│ t=0s: Client at seq 1000, then goes offline            │
└────────────────────┬────────────────────────────────────┘
                     │
                     ↓
┌─────────────────────────────────────────────────────────┐
│ t=0s-120s: Server continues broadcasting                │
│   Client offline, misses messages                       │
│   Server sends: seq 1001, 1002, ..., 2200               │
│   Replay buffer now holds: seq 2101-2200 (last 100)     │
│                            ↑                             │
│                     Older messages evicted               │
└────────────────────┬────────────────────────────────────┘
                     │
                     ↓
┌─────────────────────────────────────────────────────────┐
│ t=120s: Client comes back online                        │
│   Receives: seq 2201 (next live message)                │
│   Expected: seq 1001                                    │
│   Gap detected: 1001 to 2200 (1,200 messages missing!)  │
└────────────────────┬────────────────────────────────────┘
                     │
                     ↓
┌─────────────────────────────────────────────────────────┐
│ Client requests replay                                  │
│   ws.send({                                             │
│     type: "replay",                                     │
│     data: {from: 1001, to: 2200}  // Want 1,200 msgs   │
│   })                                                     │
└────────────────────┬────────────────────────────────────┘
                     │
                     ↓
┌─────────────────────────────────────────────────────────┐
│ Server checks replay buffer                             │
│   Requested: seq 1001-2200 (1,200 messages)             │
│   Available: seq 2101-2200 (100 messages only)          │
│                                                          │
│   GetRange(1001, 2200) returns:                         │
│   [seq 2101, 2102, ..., 2200]  // Only 100 messages     │
└────────────────────┬────────────────────────────────────┘
                     │
                     ↓
┌─────────────────────────────────────────────────────────┐
│ Client receives PARTIAL replay                          │
│   Got: seq 2101-2200 (100 messages)                     │
│   Missing: seq 1001-2100 (1,100 messages)               │
│                                                          │
│   Still have irrecoverable gap!                         │
│                                                          │
│   Client detects:                                       │
│   if (firstReplayedSeq !== requestedFrom) {             │
│     // 2101 !== 1001 → Gap too old!                     │
│     console.error('Irrecoverable gap')                  │
│     doFullReconnect()  ← TRIGGER FULL RECONNECT         │
│   }                                                      │
└────────────────────┬────────────────────────────────────┘
                     │
                     ↓
                 [Continue to Full Reconnect Flow below]
```

---

### Flow 2: Full Reconnect (Clean Slate)

```
┌─────────────────────────────────────────────────────────┐
│ Step 1: Close old connection                            │
│   ws.close()                                            │
│   onClose event fires on server                         │
└────────────────────┬────────────────────────────────────┘
                     │
                     ↓
┌─────────────────────────────────────────────────────────┐
│ Server cleanup (server.go:266)                          │
│   defer func() {                                        │
│     c.conn.Close()                                      │
│     s.clients.Delete(c)                                 │
│     s.connections.Put(c)  ← Client returned to pool     │
│     <-s.connectionsSem    ← Connection slot freed       │
│     s.rateLimiter.RemoveClient(c.id)                    │
│   }                                                      │
│                                                          │
│ Result:                                                  │
│   - Replay buffer cleared                               │
│   - Sequence generator reset                            │
│   - Rate limiter state removed                          │
│   - Connection count decremented                        │
└────────────────────┬────────────────────────────────────┘
                     │
                     ↓
┌─────────────────────────────────────────────────────────┐
│ Step 2: Client clears local state                       │
│   localStorage.removeItem('lastSeq')                    │
│   lastSeq = 0                                           │
│   receivedMessages = []                                 │
│   pendingReplays.clear()                                │
│                                                          │
│   // Optionally: Clear UI state                         │
│   prices = {}                                           │
│   orders = []                                           │
└────────────────────┬────────────────────────────────────┘
                     │
                     ↓
┌─────────────────────────────────────────────────────────┐
│ Step 3: Establish new WebSocket connection              │
│   ws = new WebSocket('ws://localhost:3004/ws')          │
│                                                          │
│   Latency: 100-300ms                                    │
│   - DNS lookup: 10ms                                    │
│   - TCP handshake: 50ms                                 │
│   - WebSocket upgrade: 40ms                             │
│   - TLS (if wss://): +100ms                             │
└────────────────────┬────────────────────────────────────┘
                     │
                     ↓
┌─────────────────────────────────────────────────────────┐
│ Server creates NEW client (server.go:252)               │
│   client := s.connections.Get()  ← From pool            │
│   client.id = atomic.AddInt64(&s.clientCount, 1)        │
│   // New ID: 12345 (different from old connection)      │
│                                                          │
│   client.seqGen = NewSequenceGenerator()                │
│   // New sequence generator, counter = 0                │
│                                                          │
│   client.replayBuffer = NewReplayBuffer(100)            │
│   // New empty replay buffer                            │
└────────────────────┬────────────────────────────────────┘
                     │
                     ↓
┌─────────────────────────────────────────────────────────┐
│ Step 4: Client receives FIRST message                   │
│   Message from broadcast:                               │
│   {                                                      │
│     "seq": 1,  ← STARTS AT 1, not 2201!                 │
│     "ts": 1696284500000,                                │
│     "type": "price:update",                             │
│     "data": {"symbol": "BTCUSD", "price": 50123.45}     │
│   }                                                      │
│                                                          │
│   Client sees:                                          │
│   lastSeq = 0 (reset)                                   │
│   received seq = 1                                      │
│   1 === 0 + 1 ✓ No gap!                                 │
│   lastSeq = 1                                           │
└────────────────────┬────────────────────────────────────┘
                     │
                     ↓
┌─────────────────────────────────────────────────────────┐
│ Step 5: Client continues with NEW sequence              │
│   Receives: seq 1, 2, 3, 4, 5, ...                      │
│   (Independent from old connection's 1001-2200)         │
│                                                          │
│   What about missed messages (1001-2200)?               │
│   ❌ DISCARDED - Not recoverable                        │
│   ✅ Client only gets NEW messages from now on          │
└─────────────────────────────────────────────────────────┘
```

## 🎭 What Gets Discarded vs Preserved

### Full Reconnect: What's DISCARDED

```
Client Side:
  ❌ lastSeq tracking (reset to 0)
  ❌ All received messages (unless client persisted them)
  ❌ Pending replay requests
  ❌ Connection ID / session state
  ❌ WebSocket connection handle

Server Side (for that client):
  ❌ Old Client struct (returned to pool)
  ❌ Replay buffer (cleared)
  ❌ Sequence generator (reset)
  ❌ Rate limiter state (removed)
  ❌ Slow client detection counters

Historical Messages:
  ❌ seq 1001-2100 (evicted from buffer)
  ❌ seq 2101-2200 (buffer cleared on disconnect)
  ❌ ALL previous messages lost
```

### Full Reconnect: What's PRESERVED

```
Client Side:
  ✅ Code (same JavaScript/app)
  ✅ DOM state (if not manually cleared)
  ✅ Cookies / Auth tokens
  ✅ LocalStorage (if not manually cleared)

Server Side (global):
  ✅ NATS subscription (still active)
  ✅ Other clients' connections (unaffected)
  ✅ Server statistics
  ✅ Connection semaphore (slot freed, reusable)

New Messages:
  ✅ All FUTURE messages (starting from reconnect time)
  ✅ New sequence starting at 1
```

## 📱 Real-World Example: Trading App

### Scenario: Price Chart Display

**Before disconnect:**
```
Client showing BTC price chart:
  - Historical data: Last 100 prices (from seq 900-1000)
  - Chart displays smooth line
  - Current price: $50,000 (seq 1000)
```

**Client offline for 2 minutes:**
```
Server broadcasts 1,200 price updates:
  - seq 1001: $50,001
  - seq 1002: $50,003
  - ...
  - seq 2200: $45,500 (price dropped!)
```

**Option A: Replay succeeds (gap < 100 messages)**
```
Client reconnects, requests replay
Server sends: seq 1001-1100 (100 messages from buffer)

Client updates chart:
  - Adds 100 new price points
  - Chart shows price movement during offline period
  - Smooth transition from $50,000 → $45,500
  - User sees: "Ah, price dropped while I was offline"
```

**Option B: Full reconnect (gap too old)**
```
Client reconnects, replay fails
Server sends: NEW messages starting seq 1

Client receives:
  - seq 1: $45,500 (current price)
  - seq 2: $45,501
  - seq 3: $45,499

Chart behavior:
  - Last known price: $50,000 (from before disconnect)
  - New price: $45,500
  - Chart shows: Jump from $50,000 → $45,500
  - Missing: All price movement in between

User experience:
  ⚠️  "Price suddenly dropped!" (but it was gradual)
  ⚠️  No historical context for 2-minute gap
  ⚠️  Chart has visible discontinuity

Mitigation:
  - Fetch historical data from REST API
    GET /api/prices?from=timestamp_before_disconnect&to=now
  - Fill chart gap with REST data
  - Continue with live WebSocket updates
```

## 🛠️ When to Use Each Strategy

### Use Replay (Preferred)

```
Conditions:
  ✅ Gap detected < 100 messages
  ✅ WebSocket still connected
  ✅ Network hiccup was brief (< 10 seconds)
  ✅ Client still has valid auth token

Advantages:
  - Fast (20-50ms)
  - No data loss
  - Seamless user experience
  - Preserves sequence continuity

Code:
  if (gapSize <= 100) {
    ws.send({type: "replay", data: {from, to}})
  }
```

### Use Full Reconnect (Fallback)

```
Conditions:
  ❌ Gap > 100 messages (beyond buffer)
  ❌ WebSocket connection lost
  ❌ Offline for long time (> 1 minute)
  ❌ Server restarted (old state gone)
  ❌ Auth token expired
  ❌ Irrecoverable error detected

Disadvantages:
  - Slow (500-2000ms)
  - Data loss (missed messages)
  - Jarring user experience
  - Must refetch state from REST API

Code:
  if (gapSize > 100 || ws.readyState !== WebSocket.OPEN) {
    ws.close()
    localStorage.clear()
    reconnect()
    fetchHistoricalData() // From REST API
  }
```

## 💻 Client-Side Implementation

### Smart Reconnect Strategy

```typescript
class WebSocketClient {
  private ws: WebSocket
  private lastSeq = 0
  private reconnectAttempts = 0
  private readonly MAX_REPLAY_GAP = 100

  handleMessage(envelope: MessageEnvelope) {
    const { seq } = envelope

    // Gap detection
    if (this.lastSeq > 0 && seq !== this.lastSeq + 1) {
      const gapSize = seq - this.lastSeq - 1

      console.warn(`Gap detected: ${gapSize} messages missing`)

      // Strategy 1: Try replay (if gap small enough)
      if (gapSize <= this.MAX_REPLAY_GAP) {
        console.log('Gap recoverable, requesting replay')
        this.requestReplay(this.lastSeq + 1, seq - 1)
      }
      // Strategy 2: Full reconnect (gap too large)
      else {
        console.error('Gap too large, full reconnect needed')
        this.doFullReconnect()
        return // Don't process current message
      }
    }

    this.lastSeq = seq
    this.processMessage(envelope)
  }

  requestReplay(from: number, to: number) {
    this.ws.send(JSON.stringify({
      type: 'replay',
      data: { from, to }
    }))
  }

  doFullReconnect() {
    console.log('Performing full reconnect...')

    // 1. Close current connection
    this.ws.close()

    // 2. Clear client state
    this.lastSeq = 0
    localStorage.removeItem('lastSeq')
    this.clearUIState()

    // 3. Wait before reconnecting (exponential backoff)
    const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempts), 30000)
    this.reconnectAttempts++

    setTimeout(() => {
      // 4. Create new connection
      this.connect()

      // 5. Fetch historical data from REST API (if needed)
      this.fetchMissedData()
    }, delay)
  }

  clearUIState() {
    // Clear any cached data that's now stale
    this.prices = {}
    this.orders = []
    this.chartData = []

    // Show loading state to user
    this.showReconnecting()
  }

  async fetchMissedData() {
    // Fetch recent data from REST API to fill gap
    const response = await fetch('/api/recent-data')
    const data = await response.json()
    this.initializeUIWithData(data)
  }
}
```

### Handling Partial Replay Response

```typescript
requestReplay(from: number, to: number) {
  const requestId = `${from}-${to}`
  const expectedMessages = to - from + 1

  // Track what we requested
  this.pendingReplays.set(requestId, {
    from,
    to,
    received: 0,
    startTime: Date.now()
  })

  this.ws.send(JSON.stringify({
    type: 'replay',
    data: { from, to }
  }))

  // Set timeout for replay response
  setTimeout(() => {
    const replay = this.pendingReplays.get(requestId)
    if (!replay) return // Already completed

    if (replay.received < expectedMessages) {
      console.error(`Partial replay: got ${replay.received}/${expectedMessages}`)

      // Check if we got enough to continue
      const firstReceivedSeq = from + replay.received
      if (firstReceivedSeq > from) {
        console.warn('Gap still exists, doing full reconnect')
        this.doFullReconnect()
      }
    }
  }, 5000) // 5 second timeout for replay
}

handleReplayedMessage(envelope: MessageEnvelope) {
  // Track progress
  for (const [requestId, replay] of this.pendingReplays) {
    if (envelope.seq >= replay.from && envelope.seq <= replay.to) {
      replay.received++

      // Check if replay complete
      if (replay.received === replay.to - replay.from + 1) {
        console.log(`Replay complete: ${replay.received} messages`)
        this.pendingReplays.delete(requestId)
      }
    }
  }
}
```

## 📊 Performance Comparison

```
┌─────────────────────┬──────────┬─────────────────┬──────────────┐
│ Metric              │  Replay  │ Full Reconnect  │ Improvement  │
├─────────────────────┼──────────┼─────────────────┼──────────────┤
│ Latency             │  20-50ms │   500-2000ms    │   10-40×     │
│ Data Loss           │  None    │   Missed msgs   │   100%       │
│ Server CPU          │  0.5ms   │   5ms           │   10×        │
│ Network Overhead    │  Small   │   Large         │   ~10×       │
│ User Experience     │  Seamless│   Jarring       │   Much better│
│ State Preserved     │  Yes     │   No            │   Critical   │
└─────────────────────┴──────────┴─────────────────┴──────────────┘

Replay:
  - Client: ws.send(50 bytes) + receive 50KB
  - Server: Memory read + send
  - Time: 20-50ms

Full Reconnect:
  - Client: TCP handshake + WebSocket upgrade + fetch REST data
  - Server: New connection allocation + initialization
  - Time: 500-2000ms
```

## 🎯 Key Takeaways

### Full Reconnect Means:

1. **Close old connection completely** ✅
2. **Discard all previous state** (seq numbers, buffers, etc.) ✅
3. **Create brand new connection** ✅
4. **Start sequences from 1 again** ✅
5. **Lose all messages between disconnect and reconnect** ❌
6. **Only receive NEW messages from reconnection point forward** ✅

### When It Happens:

- Gap too large (> 100 messages)
- WebSocket connection lost completely
- Server restarted
- Auth token expired
- Irrecoverable error

### User Impact:

- **With Replay**: Smooth experience, no data loss
- **With Full Reconnect**: Visible gap, must fetch historical data from REST API

### Best Practice:

```
1st choice: Replay (if gap < 100 messages)
2nd choice: Full reconnect + REST API fetch (if gap >= 100 messages)
Last resort: Full page reload (if REST API unavailable)
```

---

**Bottom Line**: Full reconnect = start fresh from scratch. Old connection and all its state (sequences, buffers) are discarded. New connection starts at seq 1. Missed messages are lost unless fetched from REST API.
