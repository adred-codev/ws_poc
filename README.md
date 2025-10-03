# WebSocket Server Monitoring with Prometheus + Grafana

Complete monitoring setup for the WebSocket server with real-time metrics visualization.

## 📋 Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [What You Get](#what-you-get)
- [Configuration](#configuration)
- [Accessing Dashboards](#accessing-dashboards)
- [Available Metrics](#available-metrics)
- [Troubleshooting](#troubleshooting)
- [Customization](#customization)

---

## 🎯 Overview

This monitoring stack provides:
- **Prometheus**: Time-series metrics collection and storage
- **Grafana**: Beautiful real-time dashboards
- **Auto-configuration**: Pre-loaded datasources and dashboards
- **15-second updates**: Near real-time monitoring
- **15-day retention**: Historical data for trend analysis

**Architecture:**
```
WebSocket Server (go-server-2)
    ↓ /metrics endpoint
Prometheus (scrapes every 15s)
    ↓ PromQL queries
Grafana (visualizes data)
    ↓ Dashboards
You (view in browser)
```

---

## 🚀 Quick Start

### 1. Start All Services

```bash
# Start everything (NATS, WebSocket server, Prometheus, Grafana)
docker-compose up -d

# Or start just monitoring stack
docker-compose up -d prometheus grafana
```

### 2. Verify Services Are Running

```bash
docker ps | grep -E "prometheus|grafana|ws-go-2"
```

Expected output:
```
odin-grafana       grafana/grafana:latest      Up    0.0.0.0:3010->3000/tcp
odin-prometheus    prom/prometheus:latest      Up    0.0.0.0:9091->9090/tcp
odin-ws-go-2       ws_poc-ws-go-2              Up    0.0.0.0:3004->3002/tcp
```

### 3. Access Grafana Dashboard

1. Open browser: **http://localhost:3010**
2. Login:
   - Username: `admin`
   - Password: `admin`
3. Navigate to: **Dashboards → WebSocket Server - Go-2**

That's it! You should see live metrics updating every 15 seconds.

---

## 📊 What You Get

### Dashboard Panels

#### **Connection Metrics**
- **Active Connections (Gauge)**: Current number of connected clients
  - Green: < 1700
  - Yellow: 1700-2000
  - Red: > 2000
- **Connections Over Time (Graph)**: Historical connection trend

#### **Message Metrics**
- **Message Rate (Graph)**: Messages sent/received per second
- **Bandwidth (Graph)**: Bytes sent/received per second

#### **Performance Metrics**
- **CPU Usage (Graph)**: Percentage utilization
  - Green: < 70%
  - Yellow: 70-90%
  - Red: > 90%
- **Memory Usage (Graph)**: MB used vs 512MB limit
  - Green: < 410MB
  - Yellow: 410-460MB
  - Red: > 460MB
- **Goroutines (Graph)**: Active goroutine count

#### **Reliability Metrics**
- **Slow Clients**: Clients disconnected per minute (too slow to keep up)
- **Rate Limited**: Messages dropped per minute (client exceeded rate limit)
- **Replay Requests**: Gap recovery requests per minute

#### **System Health**
- **NATS Connection Status (Gauge)**:
  - Green = Connected
  - Red = Disconnected

---

## ⚙️ Configuration

### Prometheus Configuration

File: `prometheus.yml`

```yaml
global:
  scrape_interval: 15s  # How often to scrape metrics

scrape_configs:
  - job_name: 'websocket-go-2'
    static_configs:
      - targets: ['ws-go-2:3002']  # WebSocket server
    metrics_path: '/metrics'

  - job_name: 'nats'
    static_configs:
      - targets: ['nats:8222']  # NATS server
    metrics_path: '/metrics'
```

**To change scrape interval:**
1. Edit `prometheus.yml`
2. Change `scrape_interval: 15s` to desired value (e.g., `5s` for 5 seconds)
3. Restart Prometheus: `docker-compose restart prometheus`

### Grafana Configuration

**Datasource**: `grafana/provisioning/datasources/prometheus.yml`
- Auto-connects to Prometheus on startup
- No manual configuration needed

**Dashboard**: `grafana/provisioning/dashboards/websocket.json`
- Pre-loaded WebSocket monitoring dashboard
- Editable through Grafana UI

### Data Retention

**Prometheus** stores data for **15 days** by default.

To change retention:

1. Edit `docker-compose.yml`
2. Find the Prometheus service
3. Change `--storage.tsdb.retention.time=15d` to desired value
   - Examples: `7d`, `30d`, `90d`
4. Restart: `docker-compose restart prometheus`

---

## 🖥️ Accessing Dashboards

### Grafana Dashboard (Primary)

**URL**: http://localhost:3010

**Default Credentials:**
- Username: `admin`
- Password: `admin`

**First Login:**
1. You'll be prompted to change the password (optional, can skip)
2. Go to **Dashboards** (left sidebar, 4 squares icon)
3. Click **WebSocket Server - Go-2**

**Dashboard Features:**
- **Time Range Picker** (top right): Change from "Last 1 hour" to "Last 6 hours", "Last 24 hours", etc.
- **Refresh Rate** (top right): Auto-refresh every 15s
- **Zoom**: Click and drag on any graph to zoom into a time range
- **Legend**: Click series names to hide/show them
- **Panel Menu**: Click panel title → View to see full screen

### Prometheus UI (Advanced)

**URL**: http://localhost:9091

**Use Cases:**
- Query metrics directly with PromQL
- Check target health
- Debug scraping issues
- Test alert rules

**Example Queries:**

```promql
# Current active connections
ws_connections_active

# Message rate over last 5 minutes
rate(ws_messages_sent_total[5m])

# Memory usage percentage
(ws_memory_bytes / ws_memory_limit_bytes) * 100

# Connection capacity percentage
(ws_connections_active / ws_connections_max) * 100
```

---

## 📈 Available Metrics

### Connection Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `ws_connections_total` | Counter | Total connections since start |
| `ws_connections_active` | Gauge | Current active connections |
| `ws_connections_max` | Gauge | Maximum allowed connections (2184) |
| `ws_connections_failed_total` | Counter | Failed connection attempts |

### Message Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `ws_messages_sent_total` | Counter | Total messages sent to clients |
| `ws_messages_received_total` | Counter | Total messages received from clients |
| `ws_bytes_sent_total` | Counter | Total bytes sent |
| `ws_bytes_received_total` | Counter | Total bytes received |

### Reliability Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `ws_slow_clients_disconnected_total` | Counter | Clients disconnected for being too slow |
| `ws_rate_limited_messages_total` | Counter | Messages dropped due to rate limiting |
| `ws_replay_requests_total` | Counter | Message replay requests served |

### Performance Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `ws_cpu_usage_percent` | Gauge | CPU usage percentage |
| `ws_memory_bytes` | Gauge | Memory usage in bytes |
| `ws_memory_limit_bytes` | Gauge | Memory limit (from cgroup) |
| `ws_goroutines_active` | Gauge | Active goroutines |

### NATS Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `ws_nats_connected` | Gauge | NATS connection status (1=up, 0=down) |
| `ws_nats_messages_received_total` | Counter | Messages received from NATS |

---

## 🔧 Troubleshooting

### Prometheus Not Scraping Metrics

**Check Prometheus targets:**

```bash
curl -s http://localhost:9091/api/v1/targets | jq '.data.activeTargets[] | {job: .labels.job, state: .health}'
```

Expected output:
```json
{"job": "websocket-go-2", "state": "up"}
{"job": "nats", "state": "up"}
```

**If target is DOWN:**

1. Check WebSocket server is running:
   ```bash
   docker ps | grep ws-go-2
   ```

2. Check metrics endpoint is accessible:
   ```bash
   curl http://localhost:3004/metrics
   ```

3. Check Prometheus logs:
   ```bash
   docker logs odin-prometheus
   ```

### Grafana Dashboard Not Loading

**Check Grafana health:**
```bash
curl http://localhost:3010/api/health
```

**Check datasource connection:**
1. Open Grafana: http://localhost:3010
2. Go to: **Configuration** (gear icon) → **Data Sources**
3. Click **Prometheus**
4. Scroll down → Click **Save & Test**
5. Should see green "Data source is working"

**If dashboard is missing:**
1. Check provisioning directory exists:
   ```bash
   ls -la grafana/provisioning/dashboards/
   ```

2. Restart Grafana:
   ```bash
   docker-compose restart grafana
   ```

3. Check Grafana logs:
   ```bash
   docker logs odin-grafana | grep -i dashboard
   ```

### No Data Showing in Graphs

**Possible causes:**

1. **Time range too old**: Check time picker (top right). Set to "Last 5 minutes" or "Last 15 minutes"

2. **No traffic yet**: Run a stress test to generate metrics:
   ```bash
   node stress-test-high-load.cjs 100 60 go2
   ```

3. **Metrics not scraped yet**: Wait 15 seconds for next scrape

4. **Query Prometheus directly** to verify data exists:
   ```bash
   curl -s "http://localhost:9091/api/v1/query?query=ws_connections_active" | jq .
   ```

### Port Conflicts

**Error**: "Bind for 0.0.0.0:9091 failed: port is already allocated"

**Solution**: Change port in `docker-compose.yml`:

```yaml
# Original
ports:
  - "9091:9090"

# Change to
ports:
  - "9092:9090"  # Use different external port
```

Then restart:
```bash
docker-compose up -d prometheus
```

---

## 🎨 Customization

### Adding New Panels to Dashboard

1. Open Grafana: http://localhost:3010
2. Navigate to: **Dashboards → WebSocket Server - Go-2**
3. Click **Add panel** (top right)
4. Select **Add a new panel**
5. In the query box, enter metric name (e.g., `ws_goroutines_active`)
6. Customize visualization (graph type, colors, thresholds)
7. Click **Apply**
8. Click **Save dashboard** (disk icon, top right)

### Example: Add Connection Errors Panel

**Query:**
```promql
rate(ws_connections_failed_total[5m])
```

**Panel Settings:**
- Title: "Connection Failures"
- Visualization: Time series
- Unit: ops (operations per second)

### Creating Alert Rules

**Example**: Alert when connections > 2000

1. Open Grafana
2. Go to: **Alerting** (bell icon) → **Alert rules**
3. Click **New alert rule**
4. Set query:
   ```promql
   ws_connections_active > 2000
   ```
5. Set condition: Alert when above 2000
6. Configure notification channel (Slack, Email, etc.)
7. Save

### Exporting Dashboard

To share your customized dashboard:

1. Open dashboard
2. Click **Share** (top right)
3. Click **Export**
4. Choose **Export for sharing externally**
5. Click **Save to file**

This creates a JSON file you can commit to the repository.

---

## 📝 Running Stress Tests

To generate traffic and see metrics in action:

### Light Load (100 connections)
```bash
node stress-test-high-load.cjs 100 60 go2
```

### Medium Load (500 connections)
```bash
node stress-test-high-load.cjs 500 120 go2
```

### Heavy Load (2000 connections, near capacity)
```bash
node stress-test-high-load.cjs 2000 180 go2
```

**Watch the dashboard while running tests to see:**
- Connection count rise
- Message rate increase
- Memory/CPU usage climb
- Potential slow client disconnections

---

## 🔍 Monitoring Best Practices

### What to Watch

**Critical Alerts:**
- Connections > 2000 (95% capacity)
- Memory > 460MB (90% of limit)
- CPU > 90%
- NATS disconnected
- High slow client disconnect rate (>10%)

**Warning Alerts:**
- Connections > 1700 (80% capacity)
- Memory > 410MB (80% of limit)
- CPU > 70%
- High replay request rate (network issues)

### Regular Checks

**Daily:**
- Check dashboard for anomalies
- Review slow client disconnect rate
- Check memory/CPU trends

**Weekly:**
- Review capacity trends
- Plan for scaling if connections consistently > 1500

**After Deployments:**
- Monitor for 15 minutes
- Check for error rate spikes
- Verify memory doesn't leak

---

## 🚦 Health Check Endpoints

### WebSocket Server

**Health endpoint:**
```bash
curl http://localhost:3004/health | jq .
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

**Metrics endpoint:**
```bash
curl http://localhost:3004/metrics
```

Returns Prometheus-format metrics (text).

---

## 📚 Additional Resources

### Prometheus
- [Query Language (PromQL)](https://prometheus.io/docs/prometheus/latest/querying/basics/)
- [Recording Rules](https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/)
- [Alerting Rules](https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/)

### Grafana
- [Dashboard Best Practices](https://grafana.com/docs/grafana/latest/best-practices/best-practices-for-creating-dashboards/)
- [Templating](https://grafana.com/docs/grafana/latest/dashboards/variables/)
- [Alerting](https://grafana.com/docs/grafana/latest/alerting/)

### Docker Compose
- [Compose File Reference](https://docs.docker.com/compose/compose-file/)

---

## 🆘 Getting Help

**Logs:**

```bash
# WebSocket server logs
docker logs odin-ws-go-2

# Prometheus logs
docker logs odin-prometheus

# Grafana logs
docker logs odin-grafana

# All logs (follow mode)
docker-compose logs -f
```

**Restart Services:**

```bash
# Restart everything
docker-compose restart

# Restart specific service
docker-compose restart prometheus
docker-compose restart grafana
docker-compose restart ws-go-2
```

**Clean Start:**

```bash
# Stop all services
docker-compose down

# Remove volumes (deletes historical data)
docker volume rm ws_poc_prometheus_data ws_poc_grafana_data

# Start fresh
docker-compose up -d
```

---

## 🎉 Quick Reference

| Service | URL | Credentials |
|---------|-----|-------------|
| Grafana Dashboard | http://localhost:3010 | admin / admin |
| Prometheus UI | http://localhost:9091 | - |
| WebSocket Server | ws://localhost:3004/ws | - |
| WebSocket Metrics | http://localhost:3004/metrics | - |
| WebSocket Health | http://localhost:3004/health | - |
| NATS | nats://localhost:4222 | - |

**Default Ports:**
- 3010: Grafana
- 9091: Prometheus
- 3004: WebSocket Server
- 4222: NATS

**Key Files:**
- `docker-compose.yml`: Service definitions
- `prometheus.yml`: Prometheus configuration
- `grafana/provisioning/datasources/prometheus.yml`: Grafana datasource
- `grafana/provisioning/dashboards/websocket.json`: Dashboard definition

**Quick Commands:**
```bash
# Start monitoring
docker-compose up -d prometheus grafana

# Stop monitoring
docker-compose stop prometheus grafana

# View logs
docker-compose logs -f prometheus grafana

# Check status
docker ps | grep -E "prometheus|grafana"

# Run stress test
node stress-test-high-load.cjs 100 60 go2
```

---

## 📝 Notes

- Prometheus scrapes metrics every **15 seconds**
- Grafana auto-refreshes dashboard every **15 seconds**
- Metrics are retained for **15 days**
- Dashboard shows data from **last 1 hour** by default
- All configuration is automated (no manual setup required)

Enjoy your monitoring! 🚀
