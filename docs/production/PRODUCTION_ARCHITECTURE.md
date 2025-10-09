# Production WebSocket Architecture: Simple & Scalable

## Executive Summary

**Platform**: Token trading platform (200,000 users)
**Current Load**: 36,000 transactions/day (~12 tx/sec peak)
**Current Frontend**: REST API polling every 3 seconds
**Target**: Real-time WebSocket to replace polling
**Strategy**: Start simple & cheap, scale when needed

---

## 1. Reality Check: Current Load Analysis

### 1.1 Actual Transaction Volume

**36,000 transactions per day**:
```
Spread across 8 hour trading window:
  36,000 ÷ (8 × 3,600) = 1.25 tx/sec average

Peak hour (10x average):
  12.5 tx/sec

Peak minute during event (50x average):
  ~30 tx/sec

THIS IS VERY LIGHT LOAD
```

**Active tokens**: ~100 tokens with daily activity (not 10,000)

### 1.2 User Concurrency Estimates

**200,000 total users**:
```
Conservative (10% concurrent): 20,000 connections
Moderate (20% concurrent): 40,000 connections
Peak event (40% concurrent): 80,000 connections
```

### 1.3 Current Polling Behavior

**Frontend polls every 3 seconds**:
```
Per user: 0.33 requests/sec
20k concurrent users: 6,666 requests/sec to backend

WebSocket replacement: Same rate but PUSHED
  - User subscribes to channels
  - Server pushes updates when data changes
  - Not every 3 seconds, only when something actually happens
```

### 1.4 Expected Message Rates

**Replacing polling with events**:

Current polling approach:
- Client polls 100 tokens every 3 seconds = 33 tokens/sec
- 95% of polls return "no change"
- Wasteful but simple

WebSocket approach (event-driven):
- Only send updates when token data actually changes
- 12.5 tx/sec → affects 12.5 tokens/sec
- But each token update goes to ALL subscribed clients

**Realistic message rates**:

| Scenario | Updates/sec | Subscribed Users/Token | Msgs/sec Total |
|----------|-------------|------------------------|----------------|
| **Light trading** | 5 tx/sec | 200 avg | 1,000 msg/sec |
| **Normal hours** | 12 tx/sec | 200 avg | 2,400 msg/sec |
| **Peak event** | 30 tx/sec | 1,000 avg | 30,000 msg/sec |

**Per-user message rate**:
- Casual (watching trending list): 0.2 msg/sec (global updates only)
- Active (5-10 favorites): 1-2 msg/sec
- Trader (watching 1 hot token): 5-10 msg/sec

**Weighted average**: ~1 msg/sec per user (vs 0.33 requests/sec polling)

---

## 2. Simple Production Architecture

### 2.1 Overview (Start Here)

```
┌────────────────────────────────────────────────────────┐
│             CLIENTS (200k users)                        │
│          20k-40k concurrent WebSocket                   │
└─────────────────────┬──────────────────────────────────┘
                      │
                      ▼
┌────────────────────────────────────────────────────────┐
│         Cloud TCP Load Balancer                         │
│         Session affinity: CLIENT_IP                     │
└─────────────────────┬──────────────────────────────────┘
                      │
                      ▼
┌────────────────────────────────────────────────────────┐
│    Managed Instance Group: ws-go (auto-scale 2-5)      │
│                                                         │
│   ┌──────────────┐       ┌──────────────┐             │
│   │  ws-go-1     │       │  ws-go-2     │             │
│   │  e2-small    │       │  e2-small    │             │
│   │  10k conns   │       │  10k conns   │             │
│   └──────┬───────┘       └──────┬───────┘             │
└──────────┼──────────────────────┼──────────────────────┘
           │                      │
           └──────────┬───────────┘
                      │
                      ▼
┌────────────────────────────────────────────────────────┐
│         OPTION A: NATS (e2-micro)                       │
│         Simple pub/sub (optional)                       │
└─────────────────────┬──────────────────────────────────┘
                      │
           OR         ▼

┌────────────────────────────────────────────────────────┐
│         OPTION B: Direct Backend Polling                │
│         ws-go polls backend API every 100ms             │
└─────────────────────┬──────────────────────────────────┘
                      │
                      ▼
┌────────────────────────────────────────────────────────┐
│              Backend API (Existing)                     │
│         Processes trades, returns token updates         │
└────────────────────────────────────────────────────────┘

         ┌──────────────────────────────┐
         │  Monitoring (e2-small)       │
         │  - Prometheus                │
         │  - Grafana                   │
         │  - Loki                      │
         └──────────────────────────────┘
```

### 2.2 Component Breakdown

**ws-go instances** (2-5× e2-small):
- Each handles 10,000 connections
- Start with 2 (20k capacity)
- Auto-scale to 5 (50k capacity) if needed
- Cost: 2 × $12.23 = $24.46/month

**NATS** (optional, 1× e2-micro):
- Simple pub/sub if backend wants to push events
- Cost: $6.12/month
- Alternative: Skip NATS, ws-go just polls backend API

**Load Balancer**:
- TCP LB with session affinity
- Cost: $18/month

**Monitoring** (1× e2-small):
- Prometheus, Grafana, Loki
- Cost: $12.23/month

**Total infrastructure**: ~$61/month (before network egress)

---

## 3. Architecture Decision: NATS vs Direct Polling

### Option A: Backend → NATS → ws-go (Recommended)

**Flow**:
```
1. Trade completes in backend
2. Backend publishes to NATS: topic "token.{id}.update"
3. ws-go subscribes to "token.*" wildcard
4. ws-go fans out to subscribed WebSocket clients
```

**Pros**:
- Clean separation (backend doesn't know about WebSocket clients)
- Easy to scale (add more ws-go instances)
- Backend can publish once, reaches all instances
- Can add other consumers later (analytics, webhooks, etc.)

**Cons**:
- One more service to manage (NATS)
- Slightly more complex

**Cost**: +$6.12/month for NATS e2-micro

### Option B: ws-go → Backend API Polling (Simpler)

**Flow**:
```
1. Trade completes in backend (silent, no push)
2. ws-go polls backend API every 100ms: GET /api/tokens/updates?since=timestamp
3. Backend returns changed tokens since last poll
4. ws-go fans out to subscribed clients
```

**Pros**:
- No NATS needed (simpler)
- Uses existing backend API
- Works immediately without backend changes

**Cons**:
- Polling overhead (small, but exists)
- Harder to scale (each ws-go instance polls independently)
- Backend load increases with # of ws-go instances
- Miss events if polling interval too long

**Cost**: $0 (no additional service)

**Load**:
```
2 ws-go instances × 10 requests/sec = 20 API req/sec to backend
This is trivial compared to current 6,666 polling req/sec from clients
```

### Recommendation: **Start with Option A (NATS)**

**Why**:
- $6/month is negligible
- Cleaner architecture
- Easier to scale later
- Backend can push events in real-time
- Can remove polling from clients entirely

**Migration path**:
1. Deploy NATS (e2-micro)
2. Backend publishes to NATS when token updated
3. ws-go subscribes and forwards
4. Clients connect to WebSocket
5. Disable polling in frontend

---

## 4. Detailed Configuration

### 4.1 ws-go (WebSocket Server)

**Instance**: e2-small (2 vCPU shared, 2GB RAM)

**Capacity per instance**:
- 10,000 concurrent connections
- 10,000 msg/sec outbound (1 msg/sec per user avg)
- 32 Mbps bandwidth (10k × 400 bytes)
- CPU: 20-30% utilization
- Memory: 25-35% utilization

**docker-compose.yml**:
```yaml
services:
  ws-go:
    build:
      context: ./src
      dockerfile: Dockerfile
    ports:
      - "3004:3002"

    environment:
      # Resource limits
      - WS_CPU_LIMIT=1.8
      - WS_MEMORY_LIMIT=1879048192      # 1792MB
      - WS_MAX_CONNECTIONS=10000

      # Worker pool (light load, fewer workers)
      - WS_WORKER_POOL_SIZE=128
      - WS_WORKER_QUEUE_SIZE=12800

      # Goroutine limits
      - WS_MAX_GOROUTINES=25000

      # No rate limiting (current load is very light)
      - WS_MAX_NATS_RATE=0              # Unlimited
      - WS_MAX_BROADCAST_RATE=0         # Unlimited

      # NATS connection
      - NATS_URL=nats://nats:4222

      # Logging
      - LOG_LEVEL=info
      - LOG_FORMAT=json

    deploy:
      resources:
        limits:
          cpus: "1.8"
          memory: 1792M

    restart: always
```

**Key code changes needed**:

1. **Subscription management**:
```go
type Client struct {
    conn          *websocket.Conn
    send          chan []byte
    subscriptions map[string]bool  // Channels client is subscribed to
}

func (c *Client) subscribe(channels []string) {
    for _, ch := range channels {
        c.subscriptions[ch] = true
    }
}
```

2. **NATS wildcard subscription**:
```go
// Subscribe to all token updates
natsConn.Subscribe("token.*.update", func(msg *nats.Msg) {
    // Parse channel from subject: "token.abc123.update"
    channel := msg.Subject

    // Send to subscribed clients only
    for client := range clients {
        if client.subscriptions[channel] {
            client.send <- msg.Data
        }
    }
})

// Subscribe to global topics
natsConn.Subscribe("global.>", func(msg *nats.Msg) {
    // Send to all clients subscribed to this global topic
})
```

3. **WebSocket compression** (60% bandwidth savings):
```go
upgrader := websocket.Upgrader{
    EnableCompression: true,
}
```

### 4.2 NATS (Message Broker)

**Instance**: 1× e2-micro (single node, no cluster needed for this load)

**docker-compose.yml**:
```yaml
services:
  nats:
    image: nats:2.12-alpine
    ports:
      - "4222:4222"  # Client connections
      - "8222:8222"  # Monitoring
    command:
      - "--jetstream"
      - "--store_dir=/data"
      - "--http_port=8222"
    volumes:
      - nats_data:/data
    deploy:
      resources:
        limits:
          cpus: "0.25"
          memory: 128M

volumes:
  nats_data:
```

**Why single node**:
- Current load: 12 tx/sec peak = trivial for NATS
- Can handle 100k+ msg/sec easily
- Upgrade to 3-node cluster when load increases 10x

**JetStream** (optional):
- Persistent message queue
- Replay messages on restart
- Useful if ws-go crashes (can catch up on missed events)

### 4.3 Backend Integration

**Minimal changes needed**:

1. **Publish to NATS when token updated**:
```javascript
// After trade completes
await nats.publish(`token.${tokenId}.update`, JSON.stringify({
  id: tokenId,
  price: newPrice,
  volume: newVolume,
  holder_count: holderCount,
  // ... all token fields
}));
```

2. **Topic structure**:
```
token.{token_id}.update     - Token data changed (price, volume, etc.)
token.{token_id}.comment     - New comment posted
global.trending              - Trending list updated (every 5 sec)
global.new_listing           - New token created
```

3. **When to publish**:
- After every trade (buy/sell)
- After liquidity change (add/remove)
- After metadata update (name, socials)
- After comment posted
- Scheduled: trending list every 5 seconds

**Event payload** (full token object):
```json
{
  "id": "abc123",
  "name": "DOGE2.0",
  "ticker": "DOGE",
  "price": 0.00012345,
  "marketcap": 1234567890,
  "volume": 9876543210,
  "holder_count": 42069,
  "btc_liquidity": 123456789,
  "token_liquidity": 987654321,
  "last_action_time": "2024-01-01T00:00:00Z",
  // ... all other fields from token.ts
}
```

Size: ~500-800 bytes per update

### 4.4 Auto-Scaling Configuration

**Managed Instance Group**:
```yaml
min_instances: 2        # Baseline (20k capacity)
max_instances: 5        # Max (50k capacity)

scale_up_triggers:
  - CPU > 60% for 2 minutes
  - Connection count > 8,000 (80% capacity)

scale_down_triggers:
  - CPU < 20% for 10 minutes
  - Connection count < 4,000 (40% capacity)
```

**Why conservative**:
- Current load fits easily in 2 instances
- Unlikely to need more than 3-4 instances
- Can manually increase max to 10+ if needed

---

## 5. Client Integration

### 5.1 WebSocket Client (React)

**Connection**:
```typescript
// lib/websocket.ts
export class TokenWebSocket {
  private ws: WebSocket;
  private subscriptions = new Set<string>();

  connect(url: string, token: string) {
    this.ws = new WebSocket(`${url}?auth=${token}`);

    this.ws.onopen = () => {
      console.log('Connected');
      // Resubscribe after reconnect
      if (this.subscriptions.size > 0) {
        this.subscribe(Array.from(this.subscriptions));
      }
    };

    this.ws.onmessage = (event) => {
      const msg = JSON.parse(event.data);
      this.handleMessage(msg);
    };

    this.ws.onclose = () => {
      // Reconnect with exponential backoff
      setTimeout(() => this.connect(url, token), this.getBackoff());
    };
  }

  subscribe(channels: string[]) {
    channels.forEach(ch => this.subscriptions.add(ch));
    this.ws.send(JSON.stringify({ type: 'subscribe', channels }));
  }

  unsubscribe(channels: string[]) {
    channels.forEach(ch => this.subscriptions.delete(ch));
    this.ws.send(JSON.stringify({ type: 'unsubscribe', channels }));
  }

  private handleMessage(msg: any) {
    switch (msg.type) {
      case 'token_update':
        this.emit('token_update', msg.data);
        break;
      case 'global_update':
        this.emit('global_update', msg.data);
        break;
    }
  }
}
```

**React Hook**:
```typescript
// hooks/useTokenWebSocket.ts
export function useTokenWebSocket() {
  const [ws, setWs] = useState<TokenWebSocket | null>(null);

  useEffect(() => {
    const socket = new TokenWebSocket();
    socket.connect(WS_URL, authToken);
    setWs(socket);

    return () => socket.disconnect();
  }, []);

  return ws;
}

// Usage in component
function TokenList() {
  const ws = useTokenWebSocket();
  const [tokens, setTokens] = useState<Token[]>([]);

  useEffect(() => {
    if (!ws) return;

    // Subscribe to trending list
    ws.subscribe(['global.trending']);

    // Listen for updates
    ws.on('global_update', (data) => {
      setTokens(data.trending);
    });

    return () => ws.unsubscribe(['global.trending']);
  }, [ws]);

  return <div>{/* Render tokens */}</div>;
}
```

### 5.2 Migration Strategy (Replace Polling)

**Current (polling)**:
```typescript
// Poll every 3 seconds
useEffect(() => {
  const interval = setInterval(async () => {
    const tokens = await api.getTokens();
    setTokens(tokens);
  }, 3000);

  return () => clearInterval(interval);
}, []);
```

**New (WebSocket)**:
```typescript
// Subscribe once, receive pushes
useEffect(() => {
  ws.subscribe(['global.trending']);

  ws.on('global_update', (data) => {
    setTokens(data.trending);  // Update UI when data changes
  });

  return () => ws.unsubscribe(['global.trending']);
}, [ws]);
```

**Benefits**:
- Instant updates (no 3 second delay)
- Lower backend load (6,666 req/sec → 0)
- Better UX (real-time vs polling)
- Lower client battery usage (no constant polling)

---

## 6. Cost Analysis

### 6.1 Infrastructure Costs (Monthly)

| Component | Qty | Type | Cost |
|-----------|-----|------|------|
| ws-go | 2 | e2-small | $24.46 |
| NATS | 1 | e2-micro | $6.12 |
| Load Balancer | 1 | TCP LB | $18.00 |
| Monitoring | 1 | e2-small | $12.23 |
| Disks | 3 | 10GB SSD | $5.10 |
| **Subtotal** | - | - | **$65.91** |

### 6.2 Network Egress (Variable)

**Conservative scenario** (20k concurrent):
```
Message rate: 20k users × 1 msg/sec = 20k msg/sec
Payload size: 400 bytes avg
Bandwidth: 20k × 400 = 8 MB/sec = 20.7 TB/month

GCP pricing (us-central1):
  First 1 TB: Free
  Next 9 TB: 9 × 1024 × $0.12 = $1,105
  Next 10.7 TB: 10.7 × 1024 × $0.11 = $1,209

Total: $2,314/month
```

**WITH COMPRESSION** (60% reduction):
```
20.7 TB × 0.4 = 8.3 TB/month

First 1 TB: Free
Next 7.3 TB: 7.3 × 1024 × $0.12 = $897

Total: $897/month
```

**Total with compression**: $65.91 + $897 = **$963/month**

### 6.3 Moderate scenario (40k concurrent)

**Message rate**: 40k × 1 msg/sec = 40k msg/sec
**Bandwidth**: 16 MB/sec = 41.4 TB/month
**Compressed**: 16.6 TB/month
**Network cost**: ~$1,800/month
**Total**: $1,866/month

**Infrastructure**: Need 4 instances (add 2) = +$24.46

**Grand total**: **$1,890/month**

### 6.4 Cost Comparison

| Users | Concurrent | Instances | Network | Total | Per Connection |
|-------|------------|-----------|---------|-------|----------------|
| 200k | 20k | 2 | $897 | **$963** | $0.048 |
| 200k | 40k | 4 | $1,800 | **$1,890** | $0.047 |
| 500k | 50k | 5 | $2,250 | **$2,376** | $0.048 |
| 500k | 100k | 10 | $4,500 | **$4,622** | $0.046 |

**Cost scales linearly** with concurrent connections.

---

## 7. Scaling Path (Future Growth)

### 7.1 Current → 10x Transactions (360k tx/day)

**Load**: 125 tx/sec peak
**Changes needed**: None! Same architecture handles it
**Why**: Still very light load for NATS and ws-go
**Cost**: Same (infrastructure), higher network egress if more users

### 7.2 Current → 100x Transactions (3.6M tx/day)

**Load**: 1,250 tx/sec peak
**Changes needed**:
- Upgrade NATS to 3-node cluster (e2-small) for reliability
- Consider message aggregation (batch updates every 100ms)
- May need 5-10 ws-go instances if user base grows
**Cost**: +$50-100/month infrastructure

### 7.3 Current → 1000x Transactions (36M tx/day)

**Load**: 12,500 tx/sec peak
**Changes needed**:
- Dedicated aggregator service (n2-standard-4)
- NATS cluster (3× e2-small or managed Synadia)
- 10-15 ws-go instances
- Binary protocol (Protocol Buffers) for bandwidth
**Architecture**: Similar to original plan (for 36k tx/sec)
**Cost**: $3,000-5,000/month

### 7.4 Horizontal Scaling (Beyond Single Region)

**When**: 500k+ users, global audience, or >100k concurrent

**Multi-region**:
```
Global Load Balancer
  ├─ us-central1 (ws-go MIG: 2-10 instances)
  ├─ europe-west1 (ws-go MIG: 2-10 instances)
  └─ asia-east1 (ws-go MIG: 2-10 instances)
      ↓
  NATS Supercluster (cross-region replication)
```

**Cost**: 3x infrastructure (~$3,000-6,000/month)

---

## 8. Implementation Timeline

### Week 1: Setup Infrastructure
- [ ] Deploy 2× ws-go instances (e2-small)
- [ ] Deploy 1× NATS instance (e2-micro)
- [ ] Deploy monitoring (Prometheus, Grafana, Loki)
- [ ] Configure load balancer
- [ ] Set up Grafana dashboards

**Time**: 2-3 days
**Cost at end**: $66/month (no traffic yet)

### Week 2: Backend Integration
- [ ] Backend publishes to NATS on token updates
- [ ] Test event flow: Backend → NATS → ws-go
- [ ] Validate payload format (match token.ts model)

**Time**: 2-3 days

### Week 3: ws-go Development
- [ ] Implement subscription protocol
- [ ] Add NATS wildcard subscriptions
- [ ] Enable WebSocket compression
- [ ] Remove rate limiting (not needed for this load)
- [ ] Add metrics (connections, subscriptions, messages)
- [ ] Write unit/integration tests

**Time**: 3-5 days

### Week 4: Frontend Integration
- [ ] Build WebSocket client library
- [ ] Create React hooks
- [ ] Update token list component
- [ ] Update token detail component
- [ ] Add reconnection logic
- [ ] Test with staging backend

**Time**: 3-5 days

### Week 5: Alpha Testing (10% Users)
- [ ] Deploy to production
- [ ] Enable for 10% of users via feature flag
- [ ] Monitor metrics (connections, message rates, latency)
- [ ] Collect user feedback
- [ ] Fix bugs

**Time**: 1 week
**Expected**: 2k concurrent connections, smooth operation

### Week 6: Beta (50% Users)
- [ ] Increase to 50% of users
- [ ] Validate auto-scaling (should add 1-2 instances)
- [ ] Monitor costs

**Expected**: 10k concurrent, 3-4 instances

### Week 7: General Availability (100%)
- [ ] Roll out to all users
- [ ] Disable polling in frontend (WebSocket only)
- [ ] Celebrate! 🎉

**Expected**: 20k-40k concurrent, 2-4 instances, **$900-1,900/month**

---

## 9. Monitoring & Alerts

### 9.1 Key Metrics

**Connections**:
- `ws_connections_current` (per instance)
- `ws_connection_duration_seconds` (histogram)

**Messages**:
- `ws_messages_sent_total` (counter, per channel)
- `ws_messages_dropped_total` (counter)
- `ws_message_latency_seconds` (histogram)

**Subscriptions**:
- `ws_subscriptions_per_client` (histogram)
- `ws_subscribers_per_channel` (gauge)

**System**:
- CPU, memory, goroutines (existing)
- NATS message rate, lag

### 9.2 Dashboards

**Real-Time Operations**:
- Total concurrent connections (all instances)
- Connection rate (connects/disconnects per minute)
- Message throughput (per channel)
- Latency p50/p99

**Capacity Planning**:
- Connection count vs max capacity
- CPU/memory utilization
- Network bandwidth usage
- Cost projections

### 9.3 Alerts

| Severity | Condition | Notification |
|----------|-----------|--------------|
| **Critical** | All instances down | Page on-call |
| **Critical** | NATS unreachable | Page on-call |
| **Warning** | Any instance >90% capacity | Slack |
| **Warning** | Message drop rate >1% | Slack |
| **Info** | Auto-scale event | Log only |

---

## 10. Frequently Asked Questions

### Q: Why use WebSocket instead of improving polling?

**A**: WebSocket provides:
- **Instant updates** (vs 3 second delay)
- **Lower backend load** (0 polling requests vs 6,666/sec)
- **Better UX** (real-time feels magical)
- **Lower costs** (1 connection vs constant requests)

### Q: Can we skip NATS and have ws-go poll backend?

**A**: Yes! Options:

**Option 1 (Recommended)**: Use NATS ($6/month)
- Cleaner architecture
- Scales better
- Backend can push events

**Option 2**: ws-go polls backend API every 100ms
- Simpler (no NATS)
- Free
- Works for current load
- Harder to scale later

**Verdict**: Use NATS. $6/month is negligible.

### Q: What if we get 10x more transactions overnight?

**A**: Architecture handles it easily.

Current: 12 tx/sec peak
10x: 120 tx/sec peak
**Still very light** for NATS (can handle 100k+ msg/sec)

No changes needed until 100x growth (1,200 tx/sec).

### Q: How do we reduce network egress costs?

**A**: Compression is critical:
- Enable WebSocket compression (60% reduction) - **DO THIS**
- Binary protocol (Protocol Buffers) - 80% total reduction
- Smarter updates (send only changed fields, not full object)

**Priority**: Enable compression immediately, consider binary later.

### Q: What happens if a ws-go instance crashes?

**A**: Graceful recovery:
1. Load balancer detects unhealthy instance (health check fails)
2. Stops routing new connections to that instance
3. Existing clients disconnect
4. Clients auto-reconnect to healthy instance
5. Clients resubscribe to channels
6. Total interruption: <5 seconds

### Q: Do we need aggregation for 36k tx/day?

**A**: No! That's only 12 tx/sec peak.

Aggregation becomes useful at 10,000+ tx/sec (1000x current volume).

For now, backend just publishes each transaction directly to NATS.

---

## Summary

**Start Simple**:
- 2× ws-go (e2-small): $24/month
- 1× NATS (e2-micro): $6/month
- Load balancer: $18/month
- Monitoring: $12/month
- **Total infrastructure**: $60/month
- **Network egress** (compressed): $900/month
- **Grand total**: **$960/month**

**Handles Current Load Easily**:
- 20-40k concurrent connections
- 12 tx/sec peak (very light)
- ~100 active tokens
- Replaces polling (better UX, lower backend load)

**Scales to 1000x Growth**:
- Same architecture works up to 12,500 tx/sec
- Just add more ws-go instances (auto-scaling)
- Upgrade NATS to cluster when needed
- Add aggregation at 10,000+ tx/sec

**Next Step**: Start Week 1 - deploy infrastructure and test with staging data

<function_calls>
<invoke name="TodoWrite">
<parameter name="todos">[{"content": "Create comprehensive production architecture document", "status": "completed", "activeForm": "Creating comprehensive production architecture document"}]