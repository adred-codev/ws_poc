package metrics

import (
	"sync"
	"sync/atomic"
	"time"
)

// SimpleMetrics provides basic metrics tracking without Prometheus
type SimpleMetrics struct {
	// Connection metrics
	connectionsTotal    int64
	connectionsActive   int64
	connectionsAccepted int64
	connectionsClosed   int64
	connectionsErrors   int64

	// Message metrics
	messagesReceived  int64
	messagesSent      int64
	messagesPerSecond float64
	messageDuplicates int64

	// Error metrics
	errorsTotal   int64
	lastErrorTime int64

	// System metrics
	goroutinesCount int64
	memoryUsage     int64
	cpuUsage        float64

	// NATS metrics
	natsConnectionStatus int64 // 1=connected, 0=disconnected
	natsReconnects       int64
	natsMessages         int64

	// Internal tracking
	startTime       time.Time
	mu              sync.RWMutex
	connectionTimes []time.Duration
	messageSizes    []int
	messageLatencies []time.Duration
}

func NewSimpleMetrics() *SimpleMetrics {
	return &SimpleMetrics{
		startTime:        time.Now(),
		connectionTimes:  make([]time.Duration, 0, 1000), // Keep last 1000 connection times
		messageSizes:     make([]int, 0, 1000),           // Keep last 1000 message sizes
		messageLatencies: make([]time.Duration, 0, 1000), // Keep last 1000 latencies
	}
}

// Connection tracking
func (m *SimpleMetrics) IncrementConnections() {
	atomic.AddInt64(&m.connectionsTotal, 1)
	atomic.AddInt64(&m.connectionsAccepted, 1)
	atomic.AddInt64(&m.connectionsActive, 1)
}

func (m *SimpleMetrics) DecrementConnections() {
	atomic.AddInt64(&m.connectionsClosed, 1)
	atomic.AddInt64(&m.connectionsActive, -1)
}

func (m *SimpleMetrics) RecordConnectionError() {
	atomic.AddInt64(&m.connectionsErrors, 1)
	atomic.AddInt64(&m.errorsTotal, 1)
}

func (m *SimpleMetrics) RecordConnectionDuration(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Keep only last 1000 entries
	if len(m.connectionTimes) >= 1000 {
		m.connectionTimes = m.connectionTimes[1:]
	}
	m.connectionTimes = append(m.connectionTimes, duration)
}

// Message tracking
func (m *SimpleMetrics) IncrementMessagesReceived() {
	atomic.AddInt64(&m.messagesReceived, 1)
}

func (m *SimpleMetrics) IncrementMessagesSent() {
	atomic.AddInt64(&m.messagesSent, 1)
}

func (m *SimpleMetrics) RecordMessageSize(size int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Keep only last 1000 entries
	if len(m.messageSizes) >= 1000 {
		m.messageSizes = m.messageSizes[1:]
	}
	m.messageSizes = append(m.messageSizes, size)
}

func (m *SimpleMetrics) IncrementDuplicates() {
	atomic.AddInt64(&m.messageDuplicates, 1)
}

func (m *SimpleMetrics) UpdateMessagesPerSecond(rate float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messagesPerSecond = rate
}

// Latency tracking
func (m *SimpleMetrics) RecordMessageLatency(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Keep only last 1000 entries
	if len(m.messageLatencies) >= 1000 {
		m.messageLatencies = m.messageLatencies[1:]
	}
	m.messageLatencies = append(m.messageLatencies, duration)
}

func (m *SimpleMetrics) RecordNATSLatency(duration time.Duration) {
	// For NATS latency, we can reuse the message latency tracking
	m.RecordMessageLatency(duration)
}

// Error tracking
func (m *SimpleMetrics) RecordError(errorType string) {
	atomic.AddInt64(&m.errorsTotal, 1)
	atomic.StoreInt64(&m.lastErrorTime, time.Now().Unix())
}

// System metrics
func (m *SimpleMetrics) UpdateGoroutinesCount(count int) {
	atomic.StoreInt64(&m.goroutinesCount, int64(count))
}

func (m *SimpleMetrics) UpdateMemoryUsage(bytes uint64) {
	atomic.StoreInt64(&m.memoryUsage, int64(bytes))
}

func (m *SimpleMetrics) UpdateCPUUsage(percent float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cpuUsage = percent
}

// NATS metrics
func (m *SimpleMetrics) SetNATSConnected(connected bool) {
	if connected {
		atomic.StoreInt64(&m.natsConnectionStatus, 1)
	} else {
		atomic.StoreInt64(&m.natsConnectionStatus, 0)
	}
}

func (m *SimpleMetrics) IncrementNATSReconnects() {
	atomic.AddInt64(&m.natsReconnects, 1)
}

func (m *SimpleMetrics) IncrementNATSMessages() {
	atomic.AddInt64(&m.natsMessages, 1)
}

// Getters for current values
func (m *SimpleMetrics) GetActiveConnections() int64 {
	return atomic.LoadInt64(&m.connectionsActive)
}

func (m *SimpleMetrics) GetUptime() time.Duration {
	return time.Since(m.startTime)
}

// GetAllStats returns all metrics in a structured format
func (m *SimpleMetrics) GetAllStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Calculate averages
	avgConnectionDuration := time.Duration(0)
	if len(m.connectionTimes) > 0 {
		total := time.Duration(0)
		for _, d := range m.connectionTimes {
			total += d
		}
		avgConnectionDuration = total / time.Duration(len(m.connectionTimes))
	}

	avgMessageSize := 0.0
	if len(m.messageSizes) > 0 {
		total := 0
		for _, size := range m.messageSizes {
			total += size
		}
		avgMessageSize = float64(total) / float64(len(m.messageSizes))
	}

	avgMessageLatency := time.Duration(0)
	if len(m.messageLatencies) > 0 {
		total := time.Duration(0)
		for _, d := range m.messageLatencies {
			total += d
		}
		avgMessageLatency = total / time.Duration(len(m.messageLatencies))
	}

	return map[string]interface{}{
		"connections": map[string]interface{}{
			"total":               atomic.LoadInt64(&m.connectionsTotal),
			"active":              atomic.LoadInt64(&m.connectionsActive),
			"accepted":            atomic.LoadInt64(&m.connectionsAccepted),
			"closed":              atomic.LoadInt64(&m.connectionsClosed),
			"errors":              atomic.LoadInt64(&m.connectionsErrors),
			"avg_duration_seconds": avgConnectionDuration.Seconds(),
		},
		"messages": map[string]interface{}{
			"received":        atomic.LoadInt64(&m.messagesReceived),
			"sent":            atomic.LoadInt64(&m.messagesSent),
			"per_second":      m.messagesPerSecond,
			"duplicates":      atomic.LoadInt64(&m.messageDuplicates),
			"avg_size_bytes":  avgMessageSize,
			"avg_latency_ms":  avgMessageLatency.Milliseconds(),
		},
		"system": map[string]interface{}{
			"goroutines":   atomic.LoadInt64(&m.goroutinesCount),
			"memory_bytes": atomic.LoadInt64(&m.memoryUsage),
			"cpu_percent":  m.cpuUsage,
		},
		"nats": map[string]interface{}{
			"connected":  atomic.LoadInt64(&m.natsConnectionStatus) == 1,
			"reconnects": atomic.LoadInt64(&m.natsReconnects),
			"messages":   atomic.LoadInt64(&m.natsMessages),
		},
		"errors": map[string]interface{}{
			"total":          atomic.LoadInt64(&m.errorsTotal),
			"last_error_ts":  atomic.LoadInt64(&m.lastErrorTime),
		},
		"uptime_seconds": m.GetUptime().Seconds(),
		"timestamp":      time.Now().Unix(),
	}
}

// GetSimpleStats returns basic stats for the React client
func (m *SimpleMetrics) GetSimpleStats() map[string]interface{} {
	return map[string]interface{}{
		"connections": map[string]interface{}{
			"active": atomic.LoadInt64(&m.connectionsActive),
		},
		"system": map[string]interface{}{
			"memory": map[string]interface{}{
				"heap_alloc": atomic.LoadInt64(&m.memoryUsage),
			},
			"goroutines": atomic.LoadInt64(&m.goroutinesCount),
		},
	}
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