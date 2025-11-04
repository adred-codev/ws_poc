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

	// Disconnect tracking with categorization
	disconnectsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ws_disconnects_total",
		Help: "Total disconnections by reason and who initiated",
	}, []string{"reason", "initiated_by"})

	connectionDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ws_connection_duration_seconds",
		Help:    "Connection duration before disconnect",
		Buckets: []float64{1, 5, 10, 30, 60, 300, 600, 1800, 3600}, // 1s to 1hr
	}, []string{"reason"})

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

	droppedBroadcasts = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ws_dropped_broadcasts_total",
		Help: "Total number of broadcast tasks dropped when worker pool queue full",
	})

	// Enhanced drop tracking with categorization
	droppedBroadcastsDetailed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ws_dropped_broadcasts_detailed_total",
		Help: "Total broadcast messages dropped by channel and reason",
	}, []string{"channel", "reason"})

	clientSendBufferSize = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ws_client_send_buffer_size",
		Help:    "Distribution of client send buffer usage",
		Buckets: []float64{0, 64, 128, 256, 384, 448, 480, 496, 504, 510, 511, 512}, // Track saturation
	}, []string{"percentile"})

	slowClientAttempts = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "ws_slow_client_attempts_before_disconnect",
		Help:    "Distribution of send attempts before slow client disconnect",
		Buckets: []float64{1, 2, 3, 4, 5, 10, 20},
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

	natsMessagesDropped = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ws_nats_messages_dropped_total",
		Help: "Total number of NATS messages dropped due to backpressure",
	})

	// Dynamic capacity metrics
	capacityMaxConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ws_capacity_max_connections",
		Help: "Current dynamic maximum connections allowed",
	})

	capacityCPUThreshold = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ws_capacity_cpu_threshold_percent",
		Help: "CPU threshold for rejecting new connections",
	})

	capacityRejectionsCPU = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ws_capacity_rejections_total",
		Help: "Total connection rejections by reason",
	}, []string{"reason"})

	capacityAvailableHeadroom = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ws_capacity_headroom_percent",
		Help: "Available resource headroom (CPU and memory)",
	}, []string{"resource"})

	// Error tracking
	errorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ws_errors_total",
		Help: "Total errors by type and severity",
	}, []string{"type", "severity"})
)

func init() {
	// Register all metrics with Prometheus
	prometheus.MustRegister(connectionsTotal)
	prometheus.MustRegister(connectionsActive)
	prometheus.MustRegister(connectionsMax)
	prometheus.MustRegister(connectionsFailed)
	prometheus.MustRegister(disconnectsTotal)
	prometheus.MustRegister(connectionDuration)

	prometheus.MustRegister(messagesSent)
	prometheus.MustRegister(messagesReceived)
	prometheus.MustRegister(bytesSent)
	prometheus.MustRegister(bytesReceived)

	prometheus.MustRegister(slowClientsDisconnected)
	prometheus.MustRegister(rateLimitedMessages)
	prometheus.MustRegister(replayRequests)
	prometheus.MustRegister(droppedBroadcasts)
	prometheus.MustRegister(droppedBroadcastsDetailed)
	prometheus.MustRegister(clientSendBufferSize)
	prometheus.MustRegister(slowClientAttempts)

	prometheus.MustRegister(memoryUsageBytes)
	prometheus.MustRegister(memoryLimitBytes)
	prometheus.MustRegister(cpuUsagePercent)
	prometheus.MustRegister(goroutinesActive)

	prometheus.MustRegister(natsConnected)
	prometheus.MustRegister(natsMessagesReceived)
	prometheus.MustRegister(natsMessagesDropped)

	prometheus.MustRegister(capacityMaxConnections)
	prometheus.MustRegister(capacityCPUThreshold)
	prometheus.MustRegister(capacityRejectionsCPU)
	prometheus.MustRegister(capacityAvailableHeadroom)

	prometheus.MustRegister(errorsTotal)
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
	connectionsMax.Set(float64(m.server.config.MaxConnections))

	// Get memory limit from cgroup
	memLimit, err := getMemoryLimit()
	if err == nil && memLimit > 0 {
		memoryLimitBytes.Set(float64(memLimit))
	}

	// Collect metrics at configured interval
	ticker := time.NewTicker(m.server.config.MetricsInterval)
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

	// Worker pool metrics
	droppedBroadcasts.Set(float64(m.server.workerPool.GetDroppedTasks()))

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

// IncrementNATSDropped increments dropped NATS message counter
func IncrementNATSDropped() {
	natsMessagesDropped.Inc()
}

// UpdateCapacityMetrics updates dynamic capacity metrics
func UpdateCapacityMetrics(maxConnections int, cpuThreshold float64) {
	capacityMaxConnections.Set(float64(maxConnections))
	capacityCPUThreshold.Set(cpuThreshold)
}

// IncrementCapacityRejection records a connection rejection with reason
func IncrementCapacityRejection(reason string) {
	capacityRejectionsCPU.WithLabelValues(reason).Inc()
}

// UpdateCapacityHeadroom updates available resource headroom
func UpdateCapacityHeadroom(cpuHeadroom, memHeadroom float64) {
	capacityAvailableHeadroom.WithLabelValues("cpu").Set(cpuHeadroom)
	capacityAvailableHeadroom.WithLabelValues("memory").Set(memHeadroom)
}

// Error severity levels for metrics and logging
const (
	ErrorSeverityWarning  = "warning"  // Non-critical, service continues
	ErrorSeverityCritical = "critical" // Critical but recoverable
	ErrorSeverityFatal    = "fatal"    // Service cannot continue
)

// Error types for categorization
const (
	ErrorTypeNATS          = "nats"
	ErrorTypeJetStream     = "jetstream"
	ErrorTypeBroadcast     = "broadcast"
	ErrorTypeSerialization = "serialization"
	ErrorTypeConnection    = "connection"
	ErrorTypeHealth        = "health"
)

// RecordError tracks an error in both Prometheus and returns true to signal logging needed
func RecordError(errorType, severity string) {
	errorsTotal.WithLabelValues(errorType, severity).Inc()
}

// RecordNATSError tracks NATS-related errors
func RecordNATSError(severity string) {
	errorsTotal.WithLabelValues(ErrorTypeNATS, severity).Inc()
}

// RecordJetStreamError tracks JetStream-related errors
func RecordJetStreamError(severity string) {
	errorsTotal.WithLabelValues(ErrorTypeJetStream, severity).Inc()
}

// RecordBroadcastError tracks broadcast-related errors
func RecordBroadcastError(severity string) {
	errorsTotal.WithLabelValues(ErrorTypeBroadcast, severity).Inc()
}

// RecordSerializationError tracks serialization errors
func RecordSerializationError(severity string) {
	errorsTotal.WithLabelValues(ErrorTypeSerialization, severity).Inc()
}

// RecordConnectionError tracks connection errors
func RecordConnectionError(severity string) {
	errorsTotal.WithLabelValues(ErrorTypeConnection, severity).Inc()
}

// Disconnect reasons - standardized constants for categorization
const (
	DisconnectReasonReadError         = "read_error"          // Client stopped reading (network issue, crash)
	DisconnectReasonWriteTimeout      = "write_timeout"       // Slow client (send buffer full)
	DisconnectReasonPingTimeout       = "ping_timeout"        // Client didn't respond to ping
	DisconnectReasonRateLimitExceeded = "rate_limit_exceeded" // Client sent too many messages
	DisconnectReasonServerShutdown    = "server_shutdown"     // Graceful shutdown
	DisconnectReasonClientInitiated   = "client_initiated"    // Normal close from client
	DisconnectReasonSubscriptionError = "subscription_error"  // Invalid subscription
	DisconnectReasonSendChannelClosed = "send_channel_closed" // Server closed send channel
)

// Who initiated the disconnect
const (
	DisconnectInitiatedByClient = "client"
	DisconnectInitiatedByServer = "server"
)

// Drop reasons - why broadcast messages were dropped
const (
	DropReasonSendTimeout        = "send_timeout"        // Timed out trying to send to client
	DropReasonBufferFull         = "buffer_full"         // Client send buffer is full
	DropReasonWorkerQueueFull    = "worker_queue_full"   // Worker pool queue full
	DropReasonClientDisconnected = "client_disconnected" // Client already disconnected
)

// RecordDisconnect tracks a disconnect with reason, initiator, and duration
func RecordDisconnect(reason, initiatedBy string, duration time.Duration) {
	disconnectsTotal.WithLabelValues(reason, initiatedBy).Inc()
	connectionDuration.WithLabelValues(reason).Observe(duration.Seconds())
}

// RecordDroppedBroadcast tracks a dropped broadcast message with channel and reason
func RecordDroppedBroadcast(channel, reason string) {
	droppedBroadcastsDetailed.WithLabelValues(channel, reason).Inc()
}

// RecordSlowClientAttempt records the number of send attempts before slow client disconnect
func RecordSlowClientAttempt(attempts int) {
	slowClientAttempts.Observe(float64(attempts))
}

// RecordClientBufferSize samples a client's send buffer usage
func RecordClientBufferSize(bufferLen, bufferCap int) {
	// Calculate percentile category
	percentile := "p50" // default
	usage := float64(bufferLen) / float64(bufferCap) * 100

	if usage >= 99 {
		percentile = "p99"
	} else if usage >= 95 {
		percentile = "p95"
	} else if usage >= 90 {
		percentile = "p90"
	} else if usage >= 75 {
		percentile = "p75"
	}

	clientSendBufferSize.WithLabelValues(percentile).Observe(float64(bufferLen))
}

// handleMetrics serves Prometheus metrics at /metrics endpoint
func handleMetrics(w http.ResponseWriter, r *http.Request) {
	promhttp.Handler().ServeHTTP(w, r)
}
