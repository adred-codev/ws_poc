package metrics

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	// Connection metrics
	connectionsTotal    prometheus.Counter
	connectionsActive   prometheus.Gauge
	connectionDuration  prometheus.Histogram
	connectionsAccepted prometheus.Counter
	connectionsClosed   prometheus.Counter
	connectionsErrors   prometheus.Counter

	// Message metrics
	messagesReceived    prometheus.Counter
	messagesSent        prometheus.Counter
	messagesPerSecond   prometheus.Gauge
	messageSize         prometheus.Histogram
	messageDuplicates   prometheus.Counter

	// Latency metrics
	messageLatency prometheus.Histogram
	natsLatency    prometheus.Histogram

	// Error metrics
	errorsTotal      prometheus.Counter
	errorsByType     *prometheus.CounterVec
	lastErrorTime    prometheus.Gauge

	// System metrics
	goroutinesCount prometheus.Gauge
	memoryUsage     prometheus.Gauge
	cpuUsage        prometheus.Gauge

	// NATS metrics
	natsConnectionStatus prometheus.Gauge
	natsReconnects       prometheus.Counter
	natsMessages         prometheus.Counter

	// Internal tracking
	startTime    time.Time
	mu           sync.RWMutex
	clientsCount int64
}

func NewMetrics() *Metrics {
	m := &Metrics{
		startTime: time.Now(),

		// Connection metrics
		connectionsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name: "websocket_connections_total",
			Help: "Total number of WebSocket connections attempted",
		}),
		connectionsActive: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "websocket_connections_active",
			Help: "Number of currently active WebSocket connections",
		}),
		connectionDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "websocket_connection_duration_seconds",
			Help:    "Duration of WebSocket connections",
			Buckets: prometheus.DefBuckets,
		}),
		connectionsAccepted: promauto.NewCounter(prometheus.CounterOpts{
			Name: "websocket_connections_accepted_total",
			Help: "Total number of accepted WebSocket connections",
		}),
		connectionsClosed: promauto.NewCounter(prometheus.CounterOpts{
			Name: "websocket_connections_closed_total",
			Help: "Total number of closed WebSocket connections",
		}),
		connectionsErrors: promauto.NewCounter(prometheus.CounterOpts{
			Name: "websocket_connections_errors_total",
			Help: "Total number of WebSocket connection errors",
		}),

		// Message metrics
		messagesReceived: promauto.NewCounter(prometheus.CounterOpts{
			Name: "websocket_messages_received_total",
			Help: "Total number of messages received from clients",
		}),
		messagesSent: promauto.NewCounter(prometheus.CounterOpts{
			Name: "websocket_messages_sent_total",
			Help: "Total number of messages sent to clients",
		}),
		messagesPerSecond: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "websocket_messages_per_second",
			Help: "Current messages per second rate",
		}),
		messageSize: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "websocket_message_size_bytes",
			Help:    "Size of WebSocket messages in bytes",
			Buckets: []float64{100, 500, 1000, 2000, 5000, 10000},
		}),
		messageDuplicates: promauto.NewCounter(prometheus.CounterOpts{
			Name: "websocket_messages_duplicates_total",
			Help: "Total number of duplicate messages dropped",
		}),

		// Latency metrics
		messageLatency: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "websocket_message_latency_seconds",
			Help:    "Latency of message processing",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
		}),
		natsLatency: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "nats_message_latency_seconds",
			Help:    "Latency of NATS message processing",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
		}),

		// Error metrics
		errorsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name: "websocket_errors_total",
			Help: "Total number of errors",
		}),
		errorsByType: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "websocket_errors_by_type_total",
			Help: "Total number of errors by type",
		}, []string{"type"}),
		lastErrorTime: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "websocket_last_error_timestamp",
			Help: "Timestamp of the last error",
		}),

		// System metrics
		goroutinesCount: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "websocket_goroutines_count",
			Help: "Number of goroutines",
		}),
		memoryUsage: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "websocket_memory_usage_bytes",
			Help: "Memory usage in bytes",
		}),
		cpuUsage: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "websocket_cpu_usage_percent",
			Help: "CPU usage percentage",
		}),

		// NATS metrics
		natsConnectionStatus: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "nats_connection_status",
			Help: "NATS connection status (1=connected, 0=disconnected)",
		}),
		natsReconnects: promauto.NewCounter(prometheus.CounterOpts{
			Name: "nats_reconnects_total",
			Help: "Total number of NATS reconnections",
		}),
		natsMessages: promauto.NewCounter(prometheus.CounterOpts{
			Name: "nats_messages_total",
			Help: "Total number of NATS messages processed",
		}),
	}

	return m
}

// Connection tracking
func (m *Metrics) IncrementConnections() {
	m.connectionsTotal.Inc()
	m.connectionsAccepted.Inc()
	m.mu.Lock()
	m.clientsCount++
	m.mu.Unlock()
	m.connectionsActive.Inc()
}

func (m *Metrics) DecrementConnections() {
	m.connectionsClosed.Inc()
	m.mu.Lock()
	m.clientsCount--
	m.mu.Unlock()
	m.connectionsActive.Dec()
}

func (m *Metrics) RecordConnectionError() {
	m.connectionsErrors.Inc()
	m.errorsTotal.Inc()
	m.errorsByType.WithLabelValues("connection").Inc()
}

func (m *Metrics) RecordConnectionDuration(duration time.Duration) {
	m.connectionDuration.Observe(duration.Seconds())
}

// Message tracking
func (m *Metrics) IncrementMessagesReceived() {
	m.messagesReceived.Inc()
}

func (m *Metrics) IncrementMessagesSent() {
	m.messagesSent.Inc()
}

func (m *Metrics) RecordMessageSize(size int) {
	m.messageSize.Observe(float64(size))
}

func (m *Metrics) IncrementDuplicates() {
	m.messageDuplicates.Inc()
}

func (m *Metrics) UpdateMessagesPerSecond(rate float64) {
	m.messagesPerSecond.Set(rate)
}

// Latency tracking
func (m *Metrics) RecordMessageLatency(duration time.Duration) {
	m.messageLatency.Observe(duration.Seconds())
}

func (m *Metrics) RecordNATSLatency(duration time.Duration) {
	m.natsLatency.Observe(duration.Seconds())
}

// Error tracking
func (m *Metrics) RecordError(errorType string) {
	m.errorsTotal.Inc()
	m.errorsByType.WithLabelValues(errorType).Inc()
	m.lastErrorTime.SetToCurrentTime()
}

// System metrics
func (m *Metrics) UpdateGoroutinesCount(count int) {
	m.goroutinesCount.Set(float64(count))
}

func (m *Metrics) UpdateMemoryUsage(bytes uint64) {
	m.memoryUsage.Set(float64(bytes))
}

func (m *Metrics) UpdateCPUUsage(percent float64) {
	m.cpuUsage.Set(percent)
}

// NATS metrics
func (m *Metrics) SetNATSConnected(connected bool) {
	if connected {
		m.natsConnectionStatus.Set(1)
	} else {
		m.natsConnectionStatus.Set(0)
	}
}

func (m *Metrics) IncrementNATSReconnects() {
	m.natsReconnects.Inc()
}

func (m *Metrics) IncrementNATSMessages() {
	m.natsMessages.Inc()
}

// Getters for current values
func (m *Metrics) GetActiveConnections() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.clientsCount
}

func (m *Metrics) GetUptime() time.Duration {
	return time.Since(m.startTime)
}

// MessageRate calculates messages per second over the last interval
type MessageRateTracker struct {
	lastCount     float64
	lastTime      time.Time
	currentRate   float64
	mu           sync.RWMutex
}

func NewMessageRateTracker() *MessageRateTracker {
	return &MessageRateTracker{
		lastTime: time.Now(),
	}
}

func (mrt *MessageRateTracker) Update(currentCount float64) {
	mrt.mu.Lock()
	defer mrt.mu.Unlock()

	now := time.Now()
	timeDelta := now.Sub(mrt.lastTime).Seconds()

	if timeDelta > 0 {
		countDelta := currentCount - mrt.lastCount
		mrt.currentRate = countDelta / timeDelta
		mrt.lastCount = currentCount
		mrt.lastTime = now
	}
}

func (mrt *MessageRateTracker) GetRate() float64 {
	mrt.mu.RLock()
	defer mrt.mu.RUnlock()
	return mrt.currentRate
}