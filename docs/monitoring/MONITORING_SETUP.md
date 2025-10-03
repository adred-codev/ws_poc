# WebSocket Server Monitoring with Prometheus + Grafana

Complete monitoring setup for the WebSocket server with real-time metrics visualization.

## Quick Start

### 1. Start All Services

```bash
# Using Taskfile
task docker:up

# Or using Docker Compose directly
docker-compose up -d
```

### 2. Access Grafana Dashboard

1. Open browser: **http://localhost:3010**
2. Login:
   - Username: `admin`
   - Password: `admin`
3. Navigate to: **Dashboards → WebSocket Server - Go**

## Service URLs

| Service | URL | Credentials |
|---------|-----|-------------|
| Grafana Dashboard | http://localhost:3010 | admin / admin |
| Prometheus UI | http://localhost:9091 | - |
| WebSocket Server | ws://localhost:3004/ws | - |
| WebSocket Metrics | http://localhost:3004/metrics | - |
| WebSocket Health | http://localhost:3004/health | - |
| NATS | nats://localhost:4222 | - |

## What You Get

### Dashboard Panels

1. **Active Connections** (Gauge) - Current WebSocket connections
2. **Connections Over Time** (Graph) - Historical connection trend
3. **Message Rate** (Graph) - Messages sent/received per second
4. **Bandwidth** (Graph) - Bytes sent/received per second
5. **CPU Usage** (Graph) - Server CPU utilization
6. **Memory Usage** (Graph) - Server memory consumption
7. **Goroutines** (Graph) - Active goroutine count
8. **Reliability Metrics** (Graph) - Slow clients, rate limits, replays
9. **NATS Connection** (Gauge) - NATS connection status

## Available Metrics

### Connection Metrics
- `ws_connections_total` - Total connections since start
- `ws_connections_active` - Current active connections
- `ws_connections_max` - Maximum allowed connections
- `ws_connections_failed_total` - Failed connection attempts

### Message Metrics
- `ws_messages_sent_total` - Total messages sent to clients
- `ws_messages_received_total` - Total messages received from clients
- `ws_bytes_sent_total` - Total bytes sent
- `ws_bytes_received_total` - Total bytes received

### Performance Metrics
- `ws_cpu_usage_percent` - CPU usage percentage
- `ws_memory_bytes` - Memory usage in bytes
- `ws_memory_limit_bytes` - Memory limit (from cgroup)
- `ws_goroutines_active` - Active goroutines

### Reliability Metrics
- `ws_slow_clients_disconnected_total` - Slow client disconnections
- `ws_rate_limited_messages_total` - Rate-limited messages
- `ws_replay_requests_total` - Message replay requests

### NATS Metrics
- `ws_nats_connected` - NATS connection status (1=up, 0=down)
- `ws_nats_messages_received_total` - Messages from NATS

## Configuration

### Prometheus Configuration

File: `prometheus.yml`

```yaml
global:
  scrape_interval: 15s  # How often to scrape metrics

scrape_configs:
  - job_name: 'websocket-go'
    static_configs:
      - targets: ['ws-go:3002']
    metrics_path: '/metrics'
```

**Change scrape interval:**
1. Edit `prometheus.yml`
2. Change `scrape_interval: 15s` to desired value
3. Restart: `task docker:rebuild SERVICE=prometheus`

### Data Retention

Prometheus stores data for **15 days** by default.

To change retention:
1. Edit `docker-compose.yml`
2. Find `--storage.tsdb.retention.time=15d`
3. Change to desired value (e.g., `30d`, `90d`)
4. Restart: `task docker:rebuild SERVICE=prometheus`

## Troubleshooting

### No Data in Grafana

**Check Prometheus targets:**
```bash
task monitor:targets
```

Expected output:
```json
{"job": "websocket-go", "state": "up"}
{"job": "nats", "state": "up"}
```

**If target is DOWN:**
1. Check service is running: `task docker:ps`
2. Check metrics endpoint: `curl http://localhost:3004/metrics`
3. Check Prometheus logs: `task docker:logs:prometheus`

### Dashboard Not Loading

**Verify Grafana health:**
```bash
curl http://localhost:3010/api/health
```

**Restart Grafana:**
```bash
task docker:rebuild SERVICE=grafana
```

## Monitoring Best Practices

### Critical Alerts
- Connections > 2000 (95% capacity)
- Memory > 460MB (90% of limit)
- CPU > 90%
- NATS disconnected

### Warning Alerts
- Connections > 1700 (80% capacity)
- Memory > 410MB (80% of limit)
- CPU > 70%
- High slow client disconnect rate (>10%)

## Running Stress Tests

```bash
# Light load
task test:light

# Medium load
task test:medium

# Heavy load
task test:heavy
```

Watch the dashboard while running tests to see:
- Connection count rise
- Message rate increase
- Memory/CPU usage climb

## Health Check Endpoints

### WebSocket Server

```bash
curl http://localhost:3004/health | jq '.'
```

Returns:
```json
{
  "status": "healthy" | "degraded" | "unhealthy",
  "healthy": true,
  "checks": {
    "nats": {"status": "connected", "healthy": true},
    "capacity": {"current": 0, "max": 2184, "percentage": 0, "healthy": true},
    "memory": {"used_mb": 12.4, "limit_mb": 512, "percentage": 2.4, "healthy": true},
    "cpu": {"percentage": 1.5, "healthy": true}
  },
  "warnings": [],
  "errors": [],
  "uptime": 123.45
}
```

## Additional Resources

- [Prometheus Docs](https://prometheus.io/docs/)
- [Grafana Docs](https://grafana.com/docs/)
- [PromQL Basics](https://prometheus.io/docs/prometheus/latest/querying/basics/)
