package main

import (
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Prometheus metrics for WebSocket server
// These metrics can be scraped by Prometheus and visualized in Grafana
var (
	// Connection metrics
	connectionsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ws_connections_total",
		Help: "Total number of WebSocket connections established",
	})

	connectionsActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ws_connections_active",
		Help: "Current number of active WebSocket connections",
	})

	connectionsMax = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ws_connections_max",
		Help: "Maximum allowed WebSocket connections",
	})

	connectionsFailed = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ws_connections_failed_total",
		Help: "Total number of failed connection attempts",
	})

	// Message metrics
	messagesSent = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ws_messages_sent_total",
		Help: "Total number of messages sent to clients",
	})

	messagesReceived = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ws_messages_received_total",
		Help: "Total number of messages received from clients",
	})

	bytesSent = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ws_bytes_sent_total",
		Help: "Total number of bytes sent to clients",
	})

	bytesReceived = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ws_bytes_received_total",
		Help: "Total number of bytes received from clients",
	})

	// Reliability metrics
	slowClientsDisconnected = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ws_slow_clients_disconnected_total",
		Help: "Total number of slow clients disconnected",
	})

	rateLimitedMessages = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ws_rate_limited_messages_total",
		Help: "Total number of rate limited messages",
	})

	replayRequests = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ws_replay_requests_total",
		Help: "Total number of replay requests served",
	})

	// System metrics
	memoryUsageBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ws_memory_bytes",
		Help: "Current memory usage in bytes",
	})

	memoryLimitBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ws_memory_limit_bytes",
		Help: "Memory limit in bytes (from cgroup)",
	})

	cpuUsagePercent = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ws_cpu_usage_percent",
		Help: "Current CPU usage percentage",
	})

	goroutinesActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ws_goroutines_active",
		Help: "Current number of active goroutines",
	})

	// NATS metrics
	natsConnected = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ws_nats_connected",
		Help: "NATS connection status (1=connected, 0=disconnected)",
	})

	natsMessagesReceived = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ws_nats_messages_received_total",
		Help: "Total number of messages received from NATS",
	})
)

func init() {
	// Register all metrics with Prometheus
	prometheus.MustRegister(connectionsTotal)
	prometheus.MustRegister(connectionsActive)
	prometheus.MustRegister(connectionsMax)
	prometheus.MustRegister(connectionsFailed)

	prometheus.MustRegister(messagesSent)
	prometheus.MustRegister(messagesReceived)
	prometheus.MustRegister(bytesSent)
	prometheus.MustRegister(bytesReceived)

	prometheus.MustRegister(slowClientsDisconnected)
	prometheus.MustRegister(rateLimitedMessages)
	prometheus.MustRegister(replayRequests)

	prometheus.MustRegister(memoryUsageBytes)
	prometheus.MustRegister(memoryLimitBytes)
	prometheus.MustRegister(cpuUsagePercent)
	prometheus.MustRegister(goroutinesActive)

	prometheus.MustRegister(natsConnected)
	prometheus.MustRegister(natsMessagesReceived)
}

// MetricsCollector handles periodic collection of system metrics
type MetricsCollector struct {
	server    *Server
	stopChan  chan struct{}
	lastStats cpuStats
}

type cpuStats struct {
	lastSampleTime time.Time
	lastCPUUsage   time.Duration
}

func NewMetricsCollector(server *Server) *MetricsCollector {
	return &MetricsCollector{
		server:   server,
		stopChan: make(chan struct{}),
	}
}

// Start begins collecting metrics periodically
func (m *MetricsCollector) Start() {
	// Set static metrics
	connectionsMax.Set(float64(2184)) // From our memory calculation

	// Get memory limit from cgroup
	memLimit, err := getMemoryLimit()
	if err == nil && memLimit > 0 {
		memoryLimitBytes.Set(float64(memLimit))
	}

	// Collect metrics every 15 seconds
	ticker := time.NewTicker(15 * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.collect()
			case <-m.stopChan:
				return
			}
		}
	}()
}

// Stop stops the metrics collector
func (m *MetricsCollector) Stop() {
	close(m.stopChan)
}

// collect gathers current metrics
func (m *MetricsCollector) collect() {
	// Connection metrics
	connectionsActive.Set(float64(atomic.LoadInt64(&m.server.stats.CurrentConnections)))

	// Memory metrics
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	memoryUsageBytes.Set(float64(mem.Alloc))

	// CPU metrics (estimated)
	cpuUsagePercent.Set(m.estimateCPU())

	// Goroutine metrics
	goroutinesActive.Set(float64(runtime.NumGoroutine()))

	// NATS status
	if m.server.natsConn != nil && m.server.natsConn.IsConnected() {
		natsConnected.Set(1)
	} else {
		natsConnected.Set(0)
	}
}

// estimateCPU gets CPU usage from server stats
func (m *MetricsCollector) estimateCPU() float64 {
	// Read CPU percentage from server stats (collected by collectMetrics goroutine)
	m.server.stats.mu.RLock()
	cpuPercent := m.server.stats.CPUPercent
	m.server.stats.mu.RUnlock()
	return cpuPercent
}

// UpdateConnectionMetrics updates connection-related metrics
func UpdateConnectionMetrics(server *Server) {
	connectionsTotal.Inc()
	connectionsActive.Set(float64(atomic.LoadInt64(&server.stats.CurrentConnections)))
}

// UpdateMessageMetrics updates message-related metrics
func UpdateMessageMetrics(sent, received int64) {
	if sent > 0 {
		messagesSent.Add(float64(sent))
	}
	if received > 0 {
		messagesReceived.Add(float64(received))
	}
}

// UpdateBytesMetrics updates bytes sent/received metrics
func UpdateBytesMetrics(sent, received int64) {
	if sent > 0 {
		bytesSent.Add(float64(sent))
	}
	if received > 0 {
		bytesReceived.Add(float64(received))
	}
}

// IncrementSlowClientDisconnects increments slow client disconnect counter
func IncrementSlowClientDisconnects() {
	slowClientsDisconnected.Inc()
}

// IncrementRateLimitedMessages increments rate limited message counter
func IncrementRateLimitedMessages() {
	rateLimitedMessages.Inc()
}

// IncrementReplayRequests increments replay request counter
func IncrementReplayRequests() {
	replayRequests.Inc()
}

// IncrementNATSMessages increments NATS message counter
func IncrementNATSMessages() {
	natsMessagesReceived.Inc()
}

// handleMetrics serves Prometheus metrics at /metrics endpoint
func handleMetrics(w http.ResponseWriter, r *http.Request) {
	promhttp.Handler().ServeHTTP(w, r)
}
