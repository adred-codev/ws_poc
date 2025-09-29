package metrics

import "time"

// MetricsInterface defines the interface that metrics implementations must satisfy
type MetricsInterface interface {
	// Connection tracking
	IncrementConnections()
	DecrementConnections()
	RecordConnectionError()
	RecordConnectionDuration(duration time.Duration)
	GetActiveConnections() int64

	// Message tracking
	IncrementMessagesReceived()
	IncrementMessagesSent()
	RecordMessageSize(size int)
	IncrementDuplicates()
	UpdateMessagesPerSecond(rate float64)

	// Latency tracking
	RecordMessageLatency(duration time.Duration)
	RecordNATSLatency(duration time.Duration)

	// Error tracking
	RecordError(errorType string)

	// System metrics
	UpdateGoroutinesCount(count int)
	UpdateMemoryUsage(bytes uint64)
	UpdateCPUUsage(percent float64)

	// NATS metrics
	SetNATSConnected(connected bool)
	IncrementNATSReconnects()
	IncrementNATSMessages()

	// Getters
	GetUptime() time.Duration
}

// Ensure EnhancedMetrics implements MetricsInterface
var _ MetricsInterface = (*EnhancedMetrics)(nil)

// Implement the interface methods for EnhancedMetrics by delegating to simpleMetrics
func (em *EnhancedMetrics) IncrementConnections() {
	em.simpleMetrics.IncrementConnections()
	// Also track in connection tracker
	// Note: AddConnection will be called separately with ID and remote addr
}

func (em *EnhancedMetrics) DecrementConnections() {
	em.simpleMetrics.DecrementConnections()
}

func (em *EnhancedMetrics) RecordConnectionError() {
	em.simpleMetrics.RecordConnectionError()
}

func (em *EnhancedMetrics) RecordConnectionDuration(duration time.Duration) {
	em.simpleMetrics.RecordConnectionDuration(duration)
}

func (em *EnhancedMetrics) GetActiveConnections() int64 {
	return em.simpleMetrics.GetActiveConnections()
}

func (em *EnhancedMetrics) IncrementMessagesReceived() {
	em.simpleMetrics.IncrementMessagesReceived()
}

func (em *EnhancedMetrics) IncrementMessagesSent() {
	em.simpleMetrics.IncrementMessagesSent()
}

func (em *EnhancedMetrics) RecordMessageSize(size int) {
	em.simpleMetrics.RecordMessageSize(size)
}

func (em *EnhancedMetrics) IncrementDuplicates() {
	em.simpleMetrics.IncrementDuplicates()
}

func (em *EnhancedMetrics) UpdateMessagesPerSecond(rate float64) {
	em.simpleMetrics.UpdateMessagesPerSecond(rate)
}

func (em *EnhancedMetrics) RecordMessageLatency(duration time.Duration) {
	em.simpleMetrics.RecordMessageLatency(duration)
}

func (em *EnhancedMetrics) RecordNATSLatency(duration time.Duration) {
	em.simpleMetrics.RecordNATSLatency(duration)
}

func (em *EnhancedMetrics) RecordError(errorType string) {
	em.simpleMetrics.RecordError(errorType)
}

func (em *EnhancedMetrics) UpdateGoroutinesCount(count int) {
	em.simpleMetrics.UpdateGoroutinesCount(count)
}

func (em *EnhancedMetrics) UpdateMemoryUsage(bytes uint64) {
	em.simpleMetrics.UpdateMemoryUsage(bytes)
}

func (em *EnhancedMetrics) UpdateCPUUsage(percent float64) {
	em.simpleMetrics.UpdateCPUUsage(percent)
}

func (em *EnhancedMetrics) SetNATSConnected(connected bool) {
	em.simpleMetrics.SetNATSConnected(connected)
}

func (em *EnhancedMetrics) IncrementNATSReconnects() {
	em.simpleMetrics.IncrementNATSReconnects()
}

func (em *EnhancedMetrics) IncrementNATSMessages() {
	em.simpleMetrics.IncrementNATSMessages()
}

func (em *EnhancedMetrics) GetUptime() time.Duration {
	return time.Since(em.startTime)
}