# 🚀 Odin WebSocket PoC

**Real-time Token Price Updates with WebSocket + NATS**

This Proof of Concept demonstrates replacing polling-based price updates with real-time WebSocket connections using NATS pub/sub messaging.

## 🎯 Objectives

- **Replace polling**: 3M requests/minute → Real-time push
- **Reduce latency**: 2-60 seconds → <50ms (target <5ms)
- **Cut costs**: $3,000/month → $1,550/month (48% reduction)
- **Scale to**: 100,000+ concurrent users

## 🏗️ Architecture

```
Browser ←→ WebSocket Server ←→ NATS ←→ Price Publisher
                ↓
           Health API
```

## 📁 Project Structure

```
ws_poc/
├── POC_PLAN.md              # Detailed implementation plan
├── docker-compose.yml       # NATS server setup
├── src/
│   ├── server.js           # WebSocket + NATS server
│   ├── publisher.js        # Price data simulator
│   └── config.js           # Configuration
├── client/
│   └── index.html          # Test client interface
├── tests/
│   └── load-test.js        # Load testing
├── scripts/
│   ├── start-dev.sh        # Development startup
│   └── stop-dev.sh         # Environment cleanup
└── README.md               # This file
```

## 🚀 Quick Start

### Prerequisites

- **Docker**: For NATS server
- **Node.js 18+**: For WebSocket server
- **8GB+ RAM**: Recommended for load testing

### 1. Start Development Environment

```bash
# Make scripts executable (if needed)
chmod +x scripts/*.sh

# Start everything (NATS + WebSocket + Publisher)
./scripts/start-dev.sh
```

This will:
- ✅ Install dependencies
- 🐳 Start NATS server in Docker
- 🔌 Start WebSocket server on port 3000
- 📊 Start price publisher simulator
- 📝 Create log files

### 2. Open Test Client

```bash
# Open in browser
open client/index.html
```

**Or manually**: Open `file:///path/to/ws_poc/client/index.html`

### 3. Test Real-time Updates

1. Click **"Connect"** in the web interface
2. Watch real-time price updates for BTC, ETH, ODIN, SOL, DOGE
3. Monitor connection stats and latency

### 4. Run Load Tests

```bash
# Test 100 connections
npm run test

# Test 500 connections
node tests/load-test.js single 500 20 45000

# Run progressive test suite (100 → 500 → 1000)
node tests/load-test.js progressive
```

### 5. Stop Environment

```bash
./scripts/stop-dev.sh
```

## 🔧 Configuration

Copy `.env.example` to `.env` and customize:

```env
# NATS Configuration
NATS_URL=nats://localhost:4222

# WebSocket Server
WS_PORT=8080
HTTP_PORT=3000

# JWT Secret (change in production)
JWT_SECRET=your-super-secret-jwt-key

# Price simulation
PRICE_UPDATE_INTERVAL=2000
TOKENS=BTC,ETH,ODIN,SOL,DOGE
```

## 📊 Monitoring & Health Checks

### Health Endpoints

- **WebSocket Health**: http://localhost:3000/health
- **Server Stats**: http://localhost:3000/stats
- **NATS Monitoring**: http://localhost:8222

### Real-time Logs

```bash
# WebSocket server logs
tail -f logs/websocket-server.log

# Price publisher logs
tail -f logs/price-publisher.log
```

## 🧪 Testing Scenarios

### 1. **Basic Functionality**
- Connect/disconnect reliability
- Real-time price updates
- Auto-reconnection on network loss
- Message latency <50ms

### 2. **Load Testing**
```bash
# Light load (100 connections)
node tests/load-test.js single 100 20 30000

# Medium load (500 connections)
node tests/load-test.js single 500 25 45000

# Heavy load (1000+ connections)
node tests/load-test.js single 1000 30 60000
```

### 3. **Stress Testing**
- Connection rate limiting
- Memory usage under load
- Message throughput
- Recovery from failures

## 📈 Performance Targets

### PoC Targets (Current)
- ✅ **1,000-5,000** concurrent connections
- ✅ **<50ms** message latency
- ✅ **99%+** connection success rate
- ✅ **Auto-reconnection** with exponential backoff

### Production Targets (Scaling Path)
- 🎯 **100,000+** concurrent connections
- 🎯 **<5ms** message latency
- 🎯 **99.9%** uptime
- 🎯 **$1,550/month** infrastructure cost

## 🔄 Scaling Path

### Phase 1: PoC (Current)
- Single server + NATS Docker
- 1k-5k connections
- ~$50-100/month cost

### Phase 2: Small Scale (10k users)
- Load balancer + Redis state
- 2-3 WebSocket servers
- ~$200-300/month cost

### Phase 3: Production (100k users)
- NATS cluster + Cloud Run
- Auto-scaling infrastructure
- ~$1,550/month cost

See `POC_PLAN.md` for detailed scaling architecture.

## 🚨 Troubleshooting

### Common Issues

**NATS Connection Failed**
```bash
# Check if NATS is running
docker ps | grep nats

# Check NATS health
curl http://localhost:8222/healthz
```

**WebSocket Connection Refused**
```bash
# Check if server is running
lsof -i :3000

# Check server logs
tail -f logs/websocket-server.log
```

**High Memory Usage**
- Reduce connection count in load tests
- Check for connection leaks
- Monitor with `top` or Activity Monitor

### Performance Issues

**High Latency**
1. Check network conditions
2. Reduce message frequency
3. Optimize message size
4. Monitor CPU usage

**Connection Drops**
1. Check auto-reconnection logic
2. Review heartbeat intervals
3. Monitor network stability
4. Check server resource limits

## 🔐 Security Notes

**Development Mode**
- ⚠️ Simple JWT validation
- ⚠️ No rate limiting
- ⚠️ Basic error handling

**Production Considerations**
- 🔒 Strong JWT secrets
- 🔒 Rate limiting
- 🔒 Input validation
- 🔒 DDoS protection
- 🔒 SSL/TLS encryption

## 📚 API Reference

### WebSocket Messages

**Client → Server**
```json
{
  "type": "ping",
  "timestamp": 1640995200000
}
```

**Server → Client**
```json
{
  "type": "price:update",
  "tokenId": "BTC",
  "price": 43250.00,
  "volume24h": 125000000,
  "priceChange24h": 2.5,
  "timestamp": 1640995200000,
  "source": "simulator"
}
```

### REST Endpoints

**GET /health**
```json
{
  "status": "healthy",
  "uptime": 3600,
  "websocket": {
    "currentConnections": 1250,
    "totalConnections": 1500
  }
}
```

**GET /stats**
```json
{
  "currentConnections": 1250,
  "totalConnections": 1500,
  "messagesSent": 45000,
  "messagesReceived": 12000,
  "uptime": 3600
}
```

## 🤝 Contributing

1. Fork the repository
2. Create feature branch: `git checkout -b feature-name`
3. Make changes and test
4. Run load tests: `npm run test`
5. Submit pull request

## 📄 License

ISC License - See LICENSE file for details

---

**🎯 Ready to replace polling with real-time?** Run `./scripts/start-dev.sh` and open `client/index.html`!