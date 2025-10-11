# Implementation Plan: Production WebSocket Deployment

## Current State Assessment

### ✅ Already Implemented

**Excellent news**: The WebSocket server (`/src`) is production-ready with:

1. **JetStream Integration** (`server.go:186-327`)
   - Stream creation/management
   - Durable consumer with manual ack
   - Message replay capability
   - Rate limiting built-in

2. **Client Reliability** (`connection.go`)
   - Sequence numbers for message ordering
   - Replay buffer (100 messages per client)
   - Slow client detection & auto-disconnect
   - Connection pooling for efficiency

3. **Resource Management** (`resource_guard.go`)
   - CPU/memory monitoring
   - Dynamic throttling based on load
   - Goroutine limits
   - Emergency brakes (reject at 75% CPU)

4. **Production Features**
   - Structured logging (zerolog → Loki)
   - Prometheus metrics
   - Rate limiting (NATS consumption, broadcasts)
   - Worker pool for message fan-out
   - Health checks
   - Graceful shutdown

### 🔧 Missing Pieces (Minimal Work)

1. **Client Subscription Management**
   - Currently broadcasts to ALL clients
   - Need: Filter by client subscriptions
   - Location: `server.go:818-819` (marked as "future enhancement")
   - Effort: 1-2 days

2. **Backend Integration**
   - Backend needs to publish to NATS/JetStream
   - Effort: 1-2 days

3. **Frontend WebSocket Client**
   - Build subscription protocol
   - Effort: 2-3 days

---

## Decision: Managed NATS (Synadia Cloud)

### Why Managed vs Self-Hosted?

| Factor | Self-Hosted (e2-micro) | Synadia Cloud |
|--------|------------------------|---------------|
| **Cost** | $6/month | $0 (free tier) - $50/month |
| **Setup Time** | 2-4 hours | 5 minutes |
| **Ops Overhead** | Monitor, upgrade, debug | Zero |
| **Reliability** | Single node (no HA) | 3+ nodes, 99.99% SLA |
| **JetStream** | Manual config | Pre-configured |
| **Scaling** | Manual (add nodes) | Automatic |
| **Monitoring** | Setup Prometheus scraping | Built-in dashboards |
| **Multi-Region** | Complex setup | Click a button |

### Recommendation: **Synadia Cloud Free Tier**

**Why perfect for your use case**:
- Free tier: 10GB bandwidth/month, 1GB storage
- Your usage: 12 tx/sec × 500 bytes = 6KB/sec = 15.5GB/month
- **Fits in free tier!** (barely, but works)
- Zero ops overhead
- Production-ready from day 1
- Upgrade to paid tier when you exceed free limits

**If exceeding free tier**:
- Paid tier: $0.10/GB bandwidth
- Your cost: 15.5GB × $0.10 = **$1.55/month**
- Still cheaper than self-hosting ($6/month)

**Verdict**: Use Synadia Cloud, start on free tier

---

## JetStream Configuration

### Current Setup (Already in Code)

Your `server.go` already configures JetStream:

```go
// Stream name: "ODIN_TOKENS" (line 199)
// Subject: "odin.token.>" (line 253)
// Retention: 30 seconds (line 57)
// Max messages: 100,000 (line 58)
// Max bytes: 50MB (line 59)
```

### Recommended JetStream Topics

**Per-Token Updates**:
```
odin.token.{token_id}.update     - Price, volume, holder_count changes
odin.token.{token_id}.comment    - New comment posted
odin.token.{token_id}.metadata   - Name, socials updated
```

**Global Topics**:
```
odin.global.trending              - Top 100 trending (every 5 sec)
odin.global.new_listing           - New token created
```

**Why JetStream for this use case**:
1. **Persistence**: Messages survive NATS restart
2. **Replay**: Clients can catch up on missed messages
3. **Exactly-once**: No duplicate messages
4. **Ordering**: Per-subject message ordering guaranteed
5. **Monitoring**: Built-in stream metrics

### JetStream Stream Config

```javascript
// Backend creates stream once (or use Synadia UI)
const stream = {
  name: "ODIN_TOKENS",
  subjects: [
    "odin.token.>",      // All token updates
    "odin.global.>"      // Global updates
  ],
  retention: "limits",   // Discard oldest when limit reached
  max_age: 30_000_000_000, // 30 seconds (nanoseconds)
  max_msgs: 100_000,     // 100k messages max
  max_bytes: 52_428_800, // 50MB max
  storage: "file",       // Persistent storage
  replicas: 1            // Single replica (free tier)
}
```

**Why 30 second retention**:
- Clients should reconnect within 30s
- Longer retention = more storage = higher cost
- 30s covers network blips, not client crashes
- Client crashes → full state reload from REST API

---

## Implementation Tasks

### Phase 1: Infrastructure Setup (Week 1)

**1.1 Sign up for Synadia Cloud** (5 minutes)
- Go to https://www.synadia.com/cloud
- Create free account
- Create JetStream stream: "ODIN_TOKENS"
- Get connection URL + credentials

**1.2 Deploy ws-go to GCP** (2-3 hours)
```bash
# Create 2× e2-small instances
gcloud compute instances create ws-go-1 ws-go-2 \
  --machine-type=e2-small \
  --zone=us-central1-a

# Deploy docker-compose with updated NATS URL
NATS_URL="nats://connect.ngs.global:4222" \
NATS_CREDS="/path/to/synadia.creds" \
docker compose up -d
```

**1.3 Set up monitoring** (2-3 hours)
- Deploy Prometheus, Grafana, Loki on separate e2-small
- Configure service discovery for ws-go instances
- Import Grafana dashboards from `/docs/monitoring/`

**1.4 Configure Load Balancer** (1-2 hours)
```bash
# Create TCP load balancer
gcloud compute forwarding-rules create ws-lb \
  --ports=3004 \
  --backend-service=ws-backend

# Health check
gcloud compute health-checks create http ws-health \
  --port=3002 --path=/health
```

### Phase 2: Code Changes (Week 2)

**2.1 Add Client Subscription Management** (1-2 days)

Add to `connection.go`:
```go
type Client struct {
    // ... existing fields ...
    subscriptions map[string]bool  // NEW: channels client is subscribed to
    subMu         sync.RWMutex     // NEW: protects subscriptions map
}

func (c *Client) subscribe(channels []string) {
    c.subMu.Lock()
    defer c.subMu.Unlock()

    for _, ch := range channels {
        c.subscriptions[ch] = true
    }
}

func (c *Client) isSubscribed(channel string) bool {
    c.subMu.RLock()
    defer c.subMu.RUnlock()
    return c.subscriptions[channel]
}
```

Add to `message.go`:
```go
type SubscribeMessage struct {
    Type     string   `json:"type"`     // "subscribe"
    Channels []string `json:"channels"` // ["odin.token.abc.update", ...]
}

type UnsubscribeMessage struct {
    Type     string   `json:"type"`     // "unsubscribe"
    Channels []string `json:"channels"`
}
```

Update `server.go` NATS handler:
```go
// Current (line 253): broadcasts to ALL clients
// Change to: filter by subscription

sub, err := s.natsJS.Subscribe("odin.token.>", func(msg *nats.Msg) {
    subject := msg.Subject // "odin.token.abc123.update"

    // Count how many clients want this message
    var sentCount int64

    s.clients.Range(func(key, value interface{}) bool {
        client := value.(*Client)

        // Check if client subscribed to this channel
        if client.isSubscribed(subject) {
            select {
            case client.send <- msg.Data:
                sentCount++
            default:
                // Client too slow, already handled by existing code
            }
        }
        return true
    })

    // Metrics
    if sentCount > 0 {
        RecordMessageSent(subject, sentCount)
    }

    msg.Ack()
})
```

**2.2 Update `handleClientMessage`** (1 hour)

In `server.go:815`, implement subscribe/unsubscribe:
```go
func (s *Server) handleClientMessage(client *Client, data []byte) {
    var base struct {
        Type string `json:"type"`
    }
    if err := json.Unmarshal(data, &base); err != nil {
        return
    }

    switch base.Type {
    case "subscribe":
        var msg SubscribeMessage
        if err := json.Unmarshal(data, &msg); err != nil {
            return
        }
        client.subscribe(msg.Channels)
        s.sendSubscriptionConfirmation(client, msg.Channels)

    case "unsubscribe":
        var msg UnsubscribeMessage
        if err := json.Unmarshal(data, &msg); err != nil {
            return
        }
        client.unsubscribe(msg.Channels)

    case "replay":
        // Existing replay logic (already implemented)

    case "heartbeat":
        // Existing heartbeat logic (already implemented)
    }
}
```

**Effort**: ~8 hours total

### Phase 3: Backend Integration (Week 2)

**3.1 Backend Publishes to JetStream**

Install NATS client in backend:
```bash
npm install nats
```

Connect to Synadia Cloud:
```javascript
import { connect, StringCodec } from 'nats';

const nc = await connect({
  servers: 'nats://connect.ngs.global:4222',
  authenticator: credsAuthenticator(await Deno.readFile('./synadia.creds'))
});

const js = nc.jetstream();
const sc = StringCodec();
```

Publish after trade:
```javascript
// After trade completes
const token = await getUpdatedToken(tokenId);

await js.publish(`odin.token.${tokenId}.update`, sc.encode(JSON.stringify({
  id: token.id,
  price: token.price,
  volume: token.volume,
  holder_count: token.holder_count,
  // ... all fields from token.ts
})));
```

**When to publish**:
- After buy/sell trade
- After add/remove liquidity
- After metadata update
- After comment posted
- Scheduled: trending list every 5 seconds

**3.2 Publish Trending List** (cron job)
```javascript
// Every 5 seconds
setInterval(async () => {
  const trending = await getTrendingTokens(100);

  await js.publish('odin.global.trending', sc.encode(JSON.stringify({
    type: 'trending_update',
    data: { top_100: trending }
  })));
}, 5000);
```

**Effort**: ~8 hours total

### Phase 4: Frontend Integration (Week 3)

**4.1 WebSocket Client Library**

Create `lib/websocket-client.ts`:
```typescript
export class OdinWebSocket {
  private ws: WebSocket | null = null;
  private subscriptions = new Set<string>();
  private listeners = new Map<string, Function[]>();

  connect(url: string, token: string) {
    this.ws = new WebSocket(`${url}?auth=${token}`);

    this.ws.onopen = () => {
      console.log('Connected to WebSocket');
      // Resubscribe after reconnect
      if (this.subscriptions.size > 0) {
        this.send({
          type: 'subscribe',
          channels: Array.from(this.subscriptions)
        });
      }
    };

    this.ws.onmessage = (event) => {
      const msg = JSON.parse(event.data);

      // Handle sequence numbers (already in server response)
      if (msg.seq) {
        this.checkSequence(msg.seq);
      }

      // Emit to listeners
      this.emit(msg.type, msg.data);
    };

    this.ws.onclose = () => {
      setTimeout(() => this.connect(url, token), this.getBackoff());
    };
  }

  subscribe(channels: string[]) {
    channels.forEach(ch => this.subscriptions.add(ch));
    this.send({ type: 'subscribe', channels });
  }

  unsubscribe(channels: string[]) {
    channels.forEach(ch => this.subscriptions.delete(ch));
    this.send({ type: 'unsubscribe', channels });
  }

  private checkSequence(seq: number) {
    // If gap detected, request replay (server already supports this!)
    if (this.lastSeq && seq > this.lastSeq + 1) {
      this.send({
        type: 'replay',
        from_seq: this.lastSeq + 1,
        to_seq: seq - 1
      });
    }
    this.lastSeq = seq;
  }
}
```

**4.2 React Hook**

```typescript
export function useTokenWebSocket() {
  const [ws, setWs] = useState<OdinWebSocket | null>(null);

  useEffect(() => {
    const socket = new OdinWebSocket();
    socket.connect(WS_URL, authToken);
    setWs(socket);

    return () => socket.disconnect();
  }, []);

  return ws;
}
```

**4.3 Update Components**

Replace polling with WebSocket subscriptions:
```typescript
// Token List Page
function TokenList() {
  const ws = useTokenWebSocket();
  const [trending, setTrending] = useState<Token[]>([]);

  useEffect(() => {
    if (!ws) return;

    ws.subscribe(['odin.global.trending']);
    ws.on('trending_update', (data) => {
      setTrending(data.top_100);
    });

    return () => ws.unsubscribe(['odin.global.trending']);
  }, [ws]);

  return <div>{/* Render trending */}</div>;
}

// Token Detail Page
function TokenDetail({ tokenId }: { tokenId: string }) {
  const ws = useTokenWebSocket();
  const [token, setToken] = useState<Token | null>(null);

  useEffect(() => {
    if (!ws) return;

    ws.subscribe([`odin.token.${tokenId}.update`]);
    ws.on('token_update', (data) => {
      if (data.id === tokenId) {
        setToken(data);
      }
    });

    return () => ws.unsubscribe([`odin.token.${tokenId}.update`]);
  }, [ws, tokenId]);

  return <div>Price: {token?.price}</div>;
}
```

**Effort**: ~16 hours total

### Phase 5: Testing & Rollout (Week 4-5)

**5.1 Load Testing** (2 days)
```bash
# Test with sustained-load-test.cjs (already exists!)
TARGET_CONNECTIONS=5000 \
DURATION=300 \
WS_URL=wss://staging.example.com/ws \
node scripts/sustained-load-test.cjs
```

**5.2 Alpha Rollout (1 week)**
- Deploy to production
- Enable for 10% of users (feature flag)
- Monitor: connections, message rates, latency, errors
- Collect feedback

**5.3 Beta → GA (1 week)**
- Increase to 50%, then 100%
- Monitor costs, performance
- Disable REST polling

---

## Deployment Configuration

### docker-compose.yml Updates

```yaml
services:
  ws-go:
    environment:
      # Synadia Cloud NATS
      - NATS_URL=nats://connect.ngs.global:4222
      - NATS_CREDS_FILE=/etc/nats/synadia.creds

      # JetStream config (already in code)
      - JS_STREAM_NAME=ODIN_TOKENS
      - JS_CONSUMER_NAME=ws-server
      - JS_STREAM_MAX_AGE=30s
      - JS_STREAM_MAX_MSGS=100000
      - JS_STREAM_MAX_BYTES=52428800

      # Production settings
      - WS_MAX_CONNECTIONS=10000
      - WS_MAX_NATS_RATE=0          # Unlimited (current load is light)
      - WS_MAX_BROADCAST_RATE=0     # Unlimited

    volumes:
      - ./synadia.creds:/etc/nats/synadia.creds:ro
```

### Auto-Scaling (GCP MIG)

```yaml
min_instances: 2
max_instances: 5

scale_up:
  - CPU > 60% for 2 min
  - Connections > 8000

scale_down:
  - CPU < 20% for 10 min
  - Connections < 4000
```

---

## Cost Summary

| Component | Type | Monthly Cost |
|-----------|------|--------------|
| ws-go (2-3 instances) | e2-small | $24-36 |
| NATS (Synadia Cloud) | Free tier | $0 |
| Load Balancer | TCP LB | $18 |
| Monitoring | e2-small | $12 |
| Network egress (compressed) | ~10 TB | $900 |
| **TOTAL** | | **$954-966** |

**At scale (40k concurrent)**:
- ws-go (4 instances): $48
- NATS: $0-10 (may exceed free tier)
- Network: $1,800
- **Total**: ~$1,878

---

## Timeline Summary

| Week | Phase | Effort | Outcome |
|------|-------|--------|---------|
| 1 | Infrastructure | 8-16 hours | NATS + ws-go deployed |
| 2 | Code changes | 16-24 hours | Subscriptions working |
| 3 | Frontend | 16-24 hours | WebSocket client ready |
| 4 | Testing | 16 hours | Load tested, bugs fixed |
| 5 | Alpha rollout | 1 week | 10% users live |
| 6 | Beta → GA | 1 week | 100% users live |

**Total timeline**: 6 weeks
**Total engineering effort**: ~60-80 hours

---

## Risk Mitigation

### Risk: Synadia Cloud free tier exceeded

**Symptoms**: Bandwidth > 10GB/month

**Mitigation**:
1. Enable WebSocket compression (60% reduction) - CRITICAL
2. Upgrade to paid tier ($0.10/GB = $2-5/month)
3. If costs spiral: Self-host NATS cluster (3× e2-micro = $18/month)

### Risk: More concurrent users than expected

**Symptoms**: Need >5 instances

**Mitigation**:
1. Increase MIG max instances to 10 (50k capacity)
2. Enable connection limits per user (max 3 devices)
3. Use larger instances (n2-standard-2 with 10 Gbps network)

### Risk: Backend can't keep up with JetStream publishing

**Symptoms**: Publish latency >100ms

**Mitigation**:
1. Batch multiple updates into single message
2. Use async publishing (fire and forget)
3. Add dedicated publisher service

---

## Future Enhancements

### Adaptive Throttling for Slow Clients

**Problem**: Current implementation disconnects slow clients after 3 consecutive send failures. While effective, this can be disruptive for clients experiencing temporary network issues or device constraints.

**Solution**: Implement adaptive per-client throttling instead of immediate disconnection.

#### Implementation Overview

**Core Concept**: When a client becomes slow, reduce their message rate instead of disconnecting them.

```go
type Client struct {
    // ... existing fields ...

    // Adaptive throttling fields
    throttleRate     float64 // 0.0-1.0 (percentage of messages to send)
    throttleLevel    int32   // 0=none, 1=light, 2=medium, 3=heavy
    lastThrottleTime time.Time
}

// Throttle levels
const (
    ThrottleNone   = 0  // 100% of messages
    ThrottleLight  = 1  // 75% of messages
    ThrottleMedium = 2  // 50% of messages
    ThrottleHeavy  = 3  // 25% of messages
)
```

#### Throttling Logic

```go
func (s *Server) broadcast(message []byte) {
    s.clients.Range(func(key, value interface{}) bool {
        client := key.(*Client)

        // Get current throttle level
        throttleLevel := atomic.LoadInt32(&client.throttleLevel)

        // Determine if we should send this message
        shouldSend := true
        if throttleLevel > ThrottleNone {
            // Use message sequence number for deterministic throttling
            // This ensures consistent message delivery patterns
            msgSeq := client.seqGen.Next()

            switch throttleLevel {
            case ThrottleLight:
                shouldSend = msgSeq % 4 != 0 // Send 75%
            case ThrottleMedium:
                shouldSend = msgSeq % 2 == 0 // Send 50%
            case ThrottleHeavy:
                shouldSend = msgSeq % 4 == 0 // Send 25%
            }
        }

        if !shouldSend {
            // Skip this message but don't disconnect
            IncrementThrottledMessages()
            return true
        }

        // Attempt send with existing logic
        select {
        case client.send <- message:
            // Success - check if we can reduce throttling
            atomic.StoreInt32(&client.sendAttempts, 0)
            s.maybeReduceThrottle(client)

        default:
            // Buffer full - increase throttling instead of disconnecting
            attempts := atomic.AddInt32(&client.sendAttempts, 1)

            if attempts >= 3 {
                s.increaseThrottle(client)
                atomic.StoreInt32(&client.sendAttempts, 0) // Reset counter
            }
        }

        return true
    })
}

func (s *Server) increaseThrottle(client *Client) {
    level := atomic.LoadInt32(&client.throttleLevel)

    if level < ThrottleHeavy {
        newLevel := level + 1
        atomic.StoreInt32(&client.throttleLevel, newLevel)
        client.lastThrottleTime = time.Now()

        s.auditLogger.Warning("ClientThrottled", "Increased throttle level for slow client", map[string]interface{}{
            "clientID":      client.id,
            "throttleLevel": newLevel,
            "ratePercent":   s.getThrottleRate(newLevel),
        })

        // Send notification to client
        s.sendThrottleNotification(client, newLevel)
    } else {
        // Already at maximum throttle - disconnect as last resort
        s.auditLogger.Warning("SlowClientDisconnected", "Client too slow even at maximum throttle", map[string]interface{}{
            "clientID":      client.id,
            "throttleLevel": level,
        })

        conn := client.conn
        if conn != nil {
            closeMsg := ws.NewCloseFrameBody(ws.StatusPolicyViolation, "Client too slow even at reduced rate")
            ws.WriteFrame(conn, ws.NewCloseFrame(closeMsg))
            conn.Close()
        }

        IncrementSlowClientDisconnects()
    }
}

func (s *Server) maybeReduceThrottle(client *Client) {
    level := atomic.LoadInt32(&client.throttleLevel)

    if level == ThrottleNone {
        return // Already at full rate
    }

    // Reduce throttle after 30 seconds of successful sends
    if time.Since(client.lastThrottleTime) > 30*time.Second {
        newLevel := level - 1
        atomic.StoreInt32(&client.throttleLevel, newLevel)
        client.lastThrottleTime = time.Now()

        s.auditLogger.Info("ClientThrottleReduced", "Reduced throttle level for recovering client", map[string]interface{}{
            "clientID":      client.id,
            "throttleLevel": newLevel,
            "ratePercent":   s.getThrottleRate(newLevel),
        })

        // Send notification to client
        s.sendThrottleNotification(client, newLevel)
    }
}

func (s *Server) sendThrottleNotification(client *Client, level int32) {
    notification := map[string]interface{}{
        "type":          "throttle_update",
        "level":         level,
        "rate_percent":  s.getThrottleRate(level),
        "message":       s.getThrottleMessage(level),
    }

    if data, err := json.Marshal(notification); err == nil {
        select {
        case client.send <- data:
            // Notification sent
        default:
            // Can't send notification, client buffer full
        }
    }
}

func (s *Server) getThrottleRate(level int32) float64 {
    switch level {
    case ThrottleLight:  return 75.0
    case ThrottleMedium: return 50.0
    case ThrottleHeavy:  return 25.0
    default:             return 100.0
    }
}

func (s *Server) getThrottleMessage(level int32) string {
    switch level {
    case ThrottleLight:
        return "Your connection is slightly slow. Receiving 75% of updates."
    case ThrottleMedium:
        return "Your connection is slow. Receiving 50% of updates. Consider refreshing."
    case ThrottleHeavy:
        return "Your connection is very slow. Receiving 25% of updates. Please reconnect."
    default:
        return "Connection quality restored. Receiving all updates."
    }
}
```

#### Client-Side Handling

```typescript
// In frontend WebSocket client
ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);

  if (msg.type === 'throttle_update') {
    // Show UI notification
    showNotification({
      type: msg.level > 1 ? 'warning' : 'info',
      message: msg.message,
      ratePercent: msg.rate_percent
    });

    // Log for debugging
    console.log(`Throttle level: ${msg.level} (${msg.rate_percent}% rate)`);

    // Maybe suggest reconnection
    if (msg.level >= 3) {
      showReconnectSuggestion();
    }
  }
};
```

#### Benefits

1. **Better User Experience**
   - Clients stay connected during temporary issues
   - Gradual degradation instead of hard disconnect
   - Clear feedback to users about connection quality

2. **Resource Efficiency**
   - Reduces reconnection storms
   - Slow clients use less bandwidth automatically
   - Fast clients unaffected

3. **Fairness**
   - Prevents slow clients from wasting server resources
   - Fast clients get 100% of messages
   - Automatic recovery when client speed improves

4. **Observability**
   - New Prometheus metrics: `ws_throttled_clients_by_level`
   - Track throttle transitions
   - Identify clients with persistent issues

#### Monitoring Metrics

```go
// Add to metrics.go
var (
    throttledClientsLight = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "ws_throttled_clients_light",
        Help: "Number of clients at light throttle (75% rate)",
    })

    throttledClientsMedium = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "ws_throttled_clients_medium",
        Help: "Number of clients at medium throttle (50% rate)",
    })

    throttledClientsHeavy = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "ws_throttled_clients_heavy",
        Help: "Number of clients at heavy throttle (25% rate)",
    })

    throttleTransitions = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "ws_throttle_transitions_total",
        Help: "Number of throttle level transitions",
    }, []string{"from_level", "to_level"})
)
```

#### Implementation Effort

| Task | Effort | Priority |
|------|--------|----------|
| Core throttling logic | 4-6 hours | High |
| Client notifications | 2 hours | Medium |
| Prometheus metrics | 2 hours | Medium |
| Frontend UI handling | 3-4 hours | Medium |
| Testing & tuning | 4-6 hours | High |
| **Total** | **15-20 hours** | **Phase 2** |

#### Rollout Strategy

1. **Phase 1**: Implement with conservative defaults
   - Enable for 10% of users
   - Monitor metrics (throttle rates, disconnections)
   - Tune thresholds based on data

2. **Phase 2**: Gradual rollout
   - 25% → 50% → 100% of users
   - A/B test: throttling vs disconnection
   - Compare user retention metrics

3. **Phase 3**: Optimization
   - Tune throttle levels based on network type (4G, 5G, WiFi)
   - Consider geo-based thresholds
   - Machine learning for adaptive thresholds

#### Configuration

Add environment variables for tuning:

```yaml
# docker-compose.yml
environment:
  WS_THROTTLE_ENABLED: "true"
  WS_THROTTLE_LIGHT_PERCENT: "75"
  WS_THROTTLE_MEDIUM_PERCENT: "50"
  WS_THROTTLE_HEAVY_PERCENT: "25"
  WS_THROTTLE_RECOVERY_SECONDS: "30"
  WS_THROTTLE_MAX_LEVEL: "3"
```

#### Alternative: Priority-Based Throttling

For advanced use cases, implement priority-based message filtering:

```go
type MessagePriority int

const (
    PriorityLow    MessagePriority = 0 // Metadata updates
    PriorityMedium MessagePriority = 1 // Comment updates
    PriorityHigh   MessagePriority = 2 // Price updates
    PriorityCritical MessagePriority = 3 // Trade confirmations
)

// When throttled, drop low-priority messages first
func (s *Server) shouldSendMessage(client *Client, priority MessagePriority) bool {
    level := atomic.LoadInt32(&client.throttleLevel)

    switch level {
    case ThrottleLight:
        return priority >= PriorityMedium // Drop low priority
    case ThrottleMedium:
        return priority >= PriorityHigh // Drop low+medium
    case ThrottleHeavy:
        return priority == PriorityCritical // Only critical
    default:
        return true // Send all
    }
}
```

This ensures critical updates (like trade confirmations) always get through, even when throttled.

---

## Next Steps

1. **This week**: Sign up for Synadia Cloud (5 min)
2. **Week 1**: Deploy infrastructure (ws-go + monitoring)
3. **Week 2**: Code changes (subscriptions) + backend integration
4. **Week 3**: Frontend integration
5. **Week 4**: Testing
6. **Week 5-6**: Rollout

**Questions before starting**:
1. Do you have GCP project set up?
2. Who will handle backend integration (publish to NATS)?
3. Who will handle frontend integration (React components)?
4. What's your target launch date?

Ready to start? 🚀
