# WebSocket PoC Plan - Odin Platform
## Simple to Scale Architecture

**Objective**: Replace polling mechanism with real-time WebSocket + NATS pub/sub system

---

## Core Architecture (Minimal Viable)

```
┌─────────────┐    WebSocket    ┌──────────────┐    NATS    ┌─────────────┐
│   Browser   │ ◄─────────────► │ WS Server    │ ◄─────────► │ NATS Server │
│   Client    │                 │ (Node.js)    │             │ (Docker)    │
└─────────────┘                 └──────────────┘             └─────────────┘
                                        ▲                            ▲
                                        │                            │
                                        ▼                            ▼
                                ┌──────────────┐             ┌─────────────┐
                                │  Health API  │             │ Price Sim   │
                                │ (Express)    │             │ Publisher   │
                                └──────────────┘             └─────────────┘
```

## Problem Statement (From Analysis)

**Current Issues:**
- 3M API requests/minute (100k users × 30 polls/minute)
- 90% requests return "no changes"
- 2-60 second update delays
- $3,000/month infrastructure costs
- 200 Firebase Function instances just for polling

**Target Solution:**
- Real-time updates <5ms latency
- 90% reduction in API load
- 48% cost reduction ($1,550/month)
- Support 100k+ concurrent users

---

## PoC Components (Start Simple)

### 1. **NATS Server** (Docker)
- Single instance for PoC
- JetStream enabled (persistence ready)
- Subject hierarchy: `odin.tokens.{id}.price`
- Handles message routing and persistence

### 2. **WebSocket Server** (Node.js)
- Target: 1,000-5,000 concurrent connections (PoC scale)
- Connection management with heartbeat
- NATS subscription → WebSocket broadcast
- JWT authentication (simple validation)
- Health check endpoint

### 3. **Price Simulator** (Node.js)
- Publishes fake price updates every 1-5 seconds
- 5-10 tokens (BTC, ETH, ODIN, SOL, DOGE)
- Simulates real trading data flow
- Variable update frequencies per token

### 4. **HTML Client** (Simple)
- Connect/disconnect buttons
- Real-time price display with charts
- Auto-reconnection with exponential backoff
- Connection status indicator
- Message latency display

### 5. **Load Testing** (Artillery/Node.js)
- Test progression: 100 → 1k → 5k connections
- Measure message latency and throughput
- Connection stability over time
- Resource usage monitoring

---

## Message Format & Protocol

### WebSocket Message Structure
```typescript
interface PriceUpdate {
  type: 'price:update';
  tokenId: string;
  price: number;
  volume24h: number;
  priceChange24h: number;
  timestamp: number;
  source: 'trade' | 'scheduler';
  nonce: string; // For deduplication
}

interface ConnectionStatus {
  type: 'connection:status';
  status: 'connected' | 'reconnecting' | 'error';
  connectedClients: number;
  latency: number;
}
```

### NATS Subject Hierarchy
```
odin.tokens.{tokenId}.price     # Individual price updates
odin.tokens.{tokenId}.volume    # Volume updates
odin.tokens.batch.update        # Batch updates from scheduler
odin.market.statistics          # Market-wide statistics
odin.system.health              # System health metrics
```

---

## Scaling Path to Production

### Phase 1: PoC (This Implementation)
- **Target**: 1,000-5,000 connections
- **Latency**: <50ms (target <5ms)
- **Infrastructure**: Single server + NATS Docker
- **Estimated Cost**: ~$50-100/month
- **Validation Points**:
  - Real-time message delivery
  - Connection stability
  - Auto-reconnection works
  - Basic load handling

### Phase 2: Small Scale (10k users)
- **Target**: 10,000 connections
- **Changes**:
  - Redis for connection state management
  - Load balancer (nginx/cloud)
  - 2-3 WebSocket server instances
  - NATS clustering preparation
- **Estimated Cost**: ~$200-300/month
- **New Features**:
  - Horizontal scaling
  - Session persistence
  - Advanced monitoring

### Phase 3: Production Scale (100k users)
- **Target**: 100,000+ connections
- **Changes**:
  - NATS cluster (3 nodes) with JetStream
  - Cloud Run auto-scaling (10-20 instances)
  - Redis cluster for state
  - CDN for static assets
  - Advanced monitoring (Prometheus/Grafana)
- **Estimated Cost**: ~$1,550/month (per analysis)
- **Enterprise Features**:
  - Message persistence and replay
  - Geographic distribution
  - Advanced authentication
  - Rate limiting and DDoS protection

---

## Key Validation Points

### Performance Metrics
1. **Message Latency**: <50ms (PoC) → <5ms (Production)
2. **Connection Capacity**: 1k+ concurrent (PoC) → 100k+ (Production)
3. **Throughput**: 1k messages/sec (PoC) → 1M+ messages/sec (Production)
4. **Uptime**: 99.9% availability target

### Functional Validation
1. **Real-time Updates**: Price changes visible instantly
2. **Connection Stability**: Auto-reconnect on network issues
3. **Resource Efficiency**: Single server handles 1k+ connections
4. **Load Testing**: Performance under concurrent load
5. **Error Handling**: Graceful degradation on failures

### Cost Validation
1. **Current vs PoC**: Measure actual resource usage
2. **Scaling Economics**: Cost per connection at different scales
3. **Network Efficiency**: Bandwidth usage vs polling

---

## Project Structure

```
ws_poc/
├── POC_PLAN.md              # This document
├── docker-compose.yml       # NATS server setup
├── package.json             # Dependencies
├── .env.example            # Environment variables
├── src/
│   ├── server.js           # WebSocket + NATS server
│   ├── publisher.js        # Price data simulator
│   ├── config.js           # Configuration management
│   └── utils/
│       ├── auth.js         # JWT validation
│       ├── nats-client.js  # NATS connection wrapper
│       └── websocket.js    # WebSocket utilities
├── client/
│   ├── index.html          # Test client interface
│   ├── js/
│   │   └── websocket-client.js  # Client-side logic
│   └── css/
│       └── styles.css      # Basic styling
├── tests/
│   ├── load-test.js        # Artillery/custom load tests
│   └── unit/               # Unit tests
├── docs/
│   ├── scaling-plan.md     # Production roadmap
│   ├── api-spec.md         # WebSocket API documentation
│   └── deployment.md       # Deployment instructions
└── scripts/
    ├── start-dev.sh        # Development startup
    └── deploy.sh           # Deployment script
```

---

## Success Criteria

### Technical Success
- [ ] Handle 1,000+ concurrent WebSocket connections
- [ ] Message latency consistently <50ms
- [ ] Zero message loss during normal operation
- [ ] Auto-reconnection works reliably
- [ ] Load test passes at target capacity

### Business Success
- [ ] Demonstrate 90% reduction in API calls
- [ ] Show real-time updates vs 2-second polling delay
- [ ] Resource usage significantly lower than current
- [ ] Clear path to 100k user scale
- [ ] Cost projection validates $1,550/month target

### User Experience Success
- [ ] Instant price updates (no 2-second delay)
- [ ] Stable connections (no frequent disconnects)
- [ ] Fast initial connection (<1 second)
- [ ] Graceful handling of network issues
- [ ] Clear connection status feedback

---

## Next Steps

1. **Setup Infrastructure** (Docker + Node.js)
2. **Implement Core Server** (WebSocket + NATS)
3. **Build Price Simulator** (Fake trading data)
4. **Create Test Client** (HTML + JavaScript)
5. **Load Testing** (Progressive scaling tests)
6. **Performance Analysis** (Latency and resource metrics)
7. **Documentation** (Deployment and scaling guides)

**Timeline**: 1-2 weeks for complete PoC implementation and validation.