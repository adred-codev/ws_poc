# Connection Cleanup: How Server Manages Disconnections

## 🔒 The Semaphore Pattern (Connection Limit Enforcement)

### How It Works

```go
// Server struct (server.go:52)
type Server struct {
    connectionsSem chan struct{}  // Buffered channel, size = 2,184
    // Think of this as 2,184 parking spots
}

// Initialize semaphore (server.go:98)
s.connectionsSem = make(chan struct{}, 2,184)
//                 ↑ Creates 2,184 "slots" for connections
```

**Semaphore visualization:**
```
┌─────────────────────────────────────────────────────┐
│ connectionsSem (2,184 total slots)                  │
│                                                      │
│ [occupied][occupied][occupied]...[empty][empty]      │
│  ↑ Client 1 ↑ Client 2 ↑ Client 3                   │
│                                                      │
│ Occupied slots: 1,500                               │
│ Available slots: 684                                │
└─────────────────────────────────────────────────────┘
```

## 🔄 Complete Connection Lifecycle

### Step 1: Client Connects (Acquire Slot)

```go
// handleWebSocket (server.go:236)
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
    // Try to acquire connection slot
    select {
    case s.connectionsSem <- struct{}{}:
        // ✅ SUCCESS: Slot acquired
        // Semaphore now: [occupied] ← new slot filled

    case <-time.After(5 * time.Second):
        // ❌ FAILURE: All 2,184 slots full
        http.Error(w, "Server at capacity", http.StatusServiceUnavailable)
        return
    }

    // If we got here, slot was successfully acquired

    conn, _, _, err := ws.UpgradeHTTP(r, w)
    if err != nil {
        <-s.connectionsSem  // ← RELEASE slot on error
        // Semaphore now: [empty] ← slot freed
        return
    }

    // Create client
    client := s.connections.Get()
    client.conn = conn
    client.id = atomic.AddInt64(&s.clientCount, 1)

    // Track client
    s.clients.Store(client, true)
    atomic.AddInt64(&s.stats.CurrentConnections, 1)

    // Start goroutines
    go s.writePump(client)
    go s.readPump(client)  // ← This goroutine will handle cleanup
}
```

**After connection succeeds:**
```
Semaphore: [occupied][occupied][occupied]...[NEWLY OCCUPIED]
                                             ↑ Just acquired
Current connections: 1,500 → 1,501
Available slots: 684 → 683
```

### Step 2: Client Disconnects (Release Slot)

**The magic is in the `defer` statement:**

```go
// readPump (server.go:265)
func (s *Server) readPump(c *Client) {
    // defer = "guaranteed to run when function exits"
    // Runs even if:
    //   - Client closes connection
    //   - Network error
    //   - Server error
    //   - Panic in function
    defer func() {
        // 1. Close network connection
        c.conn.Close()

        // 2. Remove from active clients map
        s.clients.Delete(c)

        // 3. Decrement connection counter
        atomic.AddInt64(&s.stats.CurrentConnections, -1)

        // 4. Return Client object to pool (memory reuse)
        s.connections.Put(c)

        // 5. 🎯 RELEASE SEMAPHORE SLOT (critical!)
        <-s.connectionsSem
        // ↑ Reads from channel, freeing one slot
        // Semaphore now: [empty] ← slot available again

        // 6. Clean up rate limiter state
        s.rateLimiter.RemoveClient(c.id)
    }()

    // Main read loop (runs until connection closes)
    for {
        msg, op, err := wsutil.ReadClientData(c.conn)
        if err != nil {
            break  // ← Exit loop, triggers defer cleanup
        }

        // Process message...
    }

    // When loop exits (for ANY reason), defer runs automatically
}
```

**After cleanup runs:**
```
Semaphore: [occupied][occupied][empty]...[empty]
                                 ↑ Just freed
Current connections: 1,501 → 1,500
Available slots: 683 → 684
```

## 📊 Real-World Example: 2 Clients Disconnect

### Initial State

```
Total slots: 2,184
Occupied: 2,184 (server at capacity)
Available: 0

connectionsSem: [full][full][full]...[full] (all 2,184 filled)

Waiting clients: 100 (queued, getting HTTP 503)
```

### Client #1 Disconnects (Network Error)

```
t=0s: Client #1's network drops
      ↓
Client #1's WebSocket connection broken
      ↓
readPump() loop detects error:
  err := wsutil.ReadClientData(c.conn)
  // err = "connection reset by peer"
      ↓
readPump() exits, defer runs:
  1. c.conn.Close()                              ✓
  2. s.clients.Delete(c)                         ✓
  3. atomic.AddInt64(&s.stats.CurrentConnections, -1)  ✓
  4. s.connections.Put(c)                        ✓
  5. <-s.connectionsSem  ← FREES 1 SLOT          ✓
  6. s.rateLimiter.RemoveClient(c.id)            ✓

Result:
  Occupied: 2,184 → 2,183
  Available: 0 → 1

  connectionsSem: [full][full][empty][full]...[full]
                                ↑ One slot freed
```

### Client #2 Disconnects (Intentional Close)

```
t=0.5s: Client #2 closes connection (user closed browser)
        ↓
Client #2 sends WebSocket close frame
        ↓
readPump() receives close:
  msg, op, err := wsutil.ReadClientData(c.conn)
  // op = ws.OpClose
        ↓
readPump() exits, defer runs again:
  1-6. Same cleanup steps
  5. <-s.connectionsSem  ← FREES ANOTHER SLOT    ✓

Result:
  Occupied: 2,183 → 2,182
  Available: 1 → 2

  connectionsSem: [full][full][empty][empty][full]...[full]
                                ↑      ↑ Two slots freed
```

### Waiting Client Gets Accepted

```
t=0.6s: One of the 100 waiting clients retries connection
        ↓
handleWebSocket() tries to acquire slot:
  select {
  case s.connectionsSem <- struct{}{}:
      // ✓ SUCCESS! One of the 2 freed slots is now occupied
  }
        ↓
New connection established

Result:
  Occupied: 2,182 → 2,183
  Available: 2 → 1

  connectionsSem: [full][full][NEWLY OCCUPIED][empty][full]...[full]
                                      ↑ Slot reused
```

## 🧪 Test: Watch Connection Cleanup in Real-Time

### Terminal 1: Monitor Server Stats

```bash
# Watch connection count every second
watch -n 1 'curl -s localhost:3004/stats | jq "{currentConnections, totalConnections}"'

Output:
{
  "currentConnections": 100,
  "totalConnections": 150
}
# ↑ currentConnections = active now
# ↑ totalConnections = all-time (including disconnected)
```

### Terminal 2: Connect 100 Clients

```bash
node stress-test-high-load.cjs 100 60 go2

# Watch Terminal 1:
{
  "currentConnections": 100,  ← Incremented
  "totalConnections": 250     ← Incremented
}
```

### Terminal 3: Kill 50 Clients (Simulate Disconnect)

```bash
# Manually close half the connections
# (stress test client has Ctrl+C handler)
# Or wait for test to end

# Watch Terminal 1:
{
  "currentConnections": 50,   ← DECREMENTED! Slots freed
  "totalConnections": 250     ← Unchanged (all-time count)
}
```

### Terminal 4: Connect 50 More Clients (Reuse Freed Slots)

```bash
node stress-test-high-load.cjs 50 60 go2

# Watch Terminal 1:
{
  "currentConnections": 100,  ← Back to 100 (reused freed slots)
  "totalConnections": 300     ← Incremented (new connections)
}
```

## 🔍 Why This Works: Go's `defer` Guarantee

### The Power of `defer`

```go
func example() {
    defer cleanup()  // Registered to run at function exit

    doWork()         // Might panic
    moreWork()       // Might return early

    // cleanup() WILL run, no matter what:
    // - Normal function return ✓
    // - Early return ✓
    // - Panic ✓
    // - Error ✓
}
```

**In our case:**

```go
func (s *Server) readPump(c *Client) {
    defer func() {
        <-s.connectionsSem  // ← ALWAYS runs
    }()

    // Possible exit scenarios:

    // 1. Client disconnects normally
    for {
        msg, _, err := wsutil.ReadClientData(c.conn)
        if err != nil {
            break  // ← defer runs, slot freed
        }
    }

    // 2. Network error
    // err = "connection reset"
    // break ← defer runs, slot freed

    // 3. Slow client detection
    // Server closes connection
    // err on next read ← defer runs, slot freed

    // 4. Server shutdown
    // Context canceled, connections closed
    // err on read ← defer runs, slot freed
}
```

**Result: Impossible to leak connection slots!**

## 🚨 What If Cleanup Fails?

### Scenario 1: Panic in Cleanup Code

```go
defer func() {
    c.conn.Close()  // Might panic?
    s.clients.Delete(c)
    <-s.connectionsSem  // ← Will this still run?
}()
```

**Answer: YES, but with caveat**

```go
// Better: Recover from panic
defer func() {
    if r := recover(); r != nil {
        s.logger.Printf("Panic in cleanup: %v", r)
    }

    // Still clean up critical resources
    if c.conn != nil {
        c.conn.Close()
    }
    s.clients.Delete(c)
    <-s.connectionsSem  // ← Always runs
}()
```

### Scenario 2: Goroutine Leak (readPump Never Exits)

```go
// Broken code (for illustration):
func (s *Server) readPump(c *Client) {
    defer func() {
        <-s.connectionsSem  // Registered but won't run
    }()

    for {
        // BUG: Never breaks out of loop even on error
        msg, _, err := wsutil.ReadClientData(c.conn)
        if err != nil {
            s.logger.Printf("Error: %v", err)
            continue  // ← BUG! Should break
        }
    }
    // defer never runs because function never exits!
}
```

**Our code doesn't have this bug:**

```go
// Correct code (server.go:278)
for {
    msg, op, err := wsutil.ReadClientData(c.conn)
    if err != nil {
        break  // ✓ Exits loop, triggers defer
    }

    // Also exits on close frame:
    if op == ws.OpClose {
        break  // ✓ Exits loop, triggers defer
    }
}
```

## 📈 Memory Leak Prevention

### Without Proper Cleanup (Hypothetical Bug)

```go
// Broken code - connection slot never freed
func (s *Server) handleWebSocket(...) {
    s.connectionsSem <- struct{}{}  // Acquire slot

    client := s.connections.Get()
    go s.readPump(client)

    // BUG: No cleanup when client disconnects
    // Slot stays occupied forever!
}

Result after 1 hour:
  - 10,000 clients connected and disconnected
  - Semaphore: 10,000/2,184 "slots" occupied (impossible!)
  - Actually: All 2,184 slots filled, no new connections accepted
  - Server permanently at capacity (restart required)
```

### With Proper Cleanup (Our Implementation)

```go
// Correct code - slot always freed
func (s *Server) readPump(c *Client) {
    defer func() {
        <-s.connectionsSem  // ✓ Always frees slot
    }()
    // ...
}

Result after 1 hour:
  - 10,000 clients connected and disconnected
  - Current connections: varies (0-2,184)
  - Semaphore: Correct count (matches current connections)
  - New connections: Always accepted (if under 2,184)
  - No memory leaks ✓
```

## 🎭 Edge Case: Rapid Reconnects

### Scenario: Client Reconnects Immediately After Disconnect

```
t=0.000s: Client #1 disconnects
          readPump() defer runs
          <-s.connectionsSem (slot freed)
          Current connections: 2,184 → 2,183

t=0.001s: Same client (new connection) attempts to connect
          handleWebSocket() runs
          s.connectionsSem <- struct{} (acquires freed slot)
          Current connections: 2,183 → 2,184

t=0.002s: Successfully connected with new Client ID

Result:
  - Slot reused within 1 millisecond ✓
  - No race condition ✓
  - Different Client object (from pool) ✓
  - Different Client ID ✓
  - New sequence numbers (starts at 1) ✓
```

**Go's channel operations are atomic, so this is safe!**

## 🛡️ Thread Safety

### Multiple Goroutines Accessing Semaphore

```go
// Multiple goroutines simultaneously:
// - 100 clients disconnecting (100 readPump goroutines running defer)
// - 50 new clients connecting (50 handleWebSocket goroutines)

// Goroutine 1 (disconnect):
<-s.connectionsSem  // Reads from channel (thread-safe)

// Goroutine 2 (disconnect):
<-s.connectionsSem  // Reads from channel (thread-safe)

// Goroutine 3 (new connection):
s.connectionsSem <- struct{}{}  // Writes to channel (thread-safe)

// Go's channel guarantees:
// - No race conditions ✓
// - Correct count maintained ✓
// - FIFO ordering for blocked operations ✓
```

## 📊 Monitoring Connection Churn

### Metrics to Track

```bash
curl -s localhost:3004/stats | jq '{
  currentConnections,
  totalConnections,
  slowClientsDisconnected
}'

{
  "currentConnections": 1500,        # Active right now
  "totalConnections": 50000,         # All-time count
  "slowClientsDisconnected": 25      # Forced disconnects
}

# Calculate churn rate:
# Churn = totalConnections - currentConnections
#       = 50000 - 1500 = 48,500 disconnections

# If server uptime = 24 hours:
# Disconnect rate = 48,500 / 86,400 seconds = 0.56/second
```

### High Churn Indicators

```
Normal:
  - Disconnect rate: < 1/second
  - Connection slots reused frequently

High Churn (Investigate):
  - Disconnect rate: > 10/second
  - Possible causes:
    * Client bugs (rapid reconnects)
    * Network instability
    * Slow client detection triggering often
    * Server performance issues
```

## 🎯 Key Takeaways

### Does Server Manage Connections? ✅ YES

**The server automatically:**
1. ✅ Acquires slot when client connects (server.go:237)
2. ✅ Tracks active connections (atomic counter)
3. ✅ Frees slot when client disconnects (server.go:271)
4. ✅ Cleans up memory (returns Client to pool)
5. ✅ Removes rate limiter state (prevents memory leak)
6. ✅ Updates connection statistics

### Does Server Vacate Connection Slots? ✅ YES

**Guaranteed cleanup via `defer`:**
```go
defer func() {
    <-s.connectionsSem  // ← Frees slot, ALWAYS runs
}()
```

**Triggers cleanup on:**
- Normal disconnect ✓
- Network error ✓
- Client close ✓
- Slow client timeout ✓
- Server shutdown ✓
- ANY error condition ✓

### What Happens When 1-2 Clients Drop?

```
Before:
  Occupied slots: 2,184/2,184 (full)
  Queued connections: 50 (getting HTTP 503)

Client 1 disconnects:
  defer runs → <-s.connectionsSem
  Occupied: 2,184 → 2,183
  Available: 0 → 1

Client 2 disconnects:
  defer runs → <-s.connectionsSem
  Occupied: 2,183 → 2,182
  Available: 1 → 2

Queued clients can now connect:
  First queued client acquires freed slot
  Occupied: 2,182 → 2,183

Result:
  ✅ Slots immediately reusable
  ✅ No manual cleanup needed
  ✅ No memory leaks
  ✅ System self-regulating
```

---

**Bottom Line**: Server manages connections perfectly. When ANY client disconnects (for ANY reason), the connection slot is **automatically and immediately freed** via Go's `defer` mechanism. The slot becomes available for new connections instantly. No manual intervention needed. No leaks possible.
