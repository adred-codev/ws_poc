package metrics

import (
	"sync"
	"time"
)

// ConnectionInfo holds detailed information about a connection
type ConnectionInfo struct {
	ID            string
	RemoteAddr    string
	ConnectedAt   time.Time
	LastMessageAt time.Time
	MessagesSent  uint64
	MessagesRecv  uint64
	BytesSent     uint64
	BytesRecv     uint64
}

// ConnectionTracker provides accurate connection tracking
type ConnectionTracker struct {
	mu               sync.RWMutex
	connections      map[string]*ConnectionInfo
	totalConnections uint64
	peakConnections  int
}

// NewConnectionTracker creates a new connection tracker
func NewConnectionTracker() *ConnectionTracker {
	return &ConnectionTracker{
		connections: make(map[string]*ConnectionInfo),
	}
}

// AddConnection registers a new connection
func (ct *ConnectionTracker) AddConnection(id, remoteAddr string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	ct.connections[id] = &ConnectionInfo{
		ID:          id,
		RemoteAddr:  remoteAddr,
		ConnectedAt: time.Now(),
	}

	ct.totalConnections++

	// Track peak connections
	currentCount := len(ct.connections)
	if currentCount > ct.peakConnections {
		ct.peakConnections = currentCount
	}
}

// RemoveConnection removes a connection
func (ct *ConnectionTracker) RemoveConnection(id string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	delete(ct.connections, id)
}

// UpdateConnectionStats updates statistics for a connection
func (ct *ConnectionTracker) UpdateConnectionStats(id string, sent bool, bytes uint64) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if conn, exists := ct.connections[id]; exists {
		conn.LastMessageAt = time.Now()
		if sent {
			conn.MessagesSent++
			conn.BytesSent += bytes
		} else {
			conn.MessagesRecv++
			conn.BytesRecv += bytes
		}
	}
}

// GetActiveCount returns the current number of active connections
func (ct *ConnectionTracker) GetActiveCount() int {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	return len(ct.connections)
}

// GetConnectionStats returns detailed connection statistics
func (ct *ConnectionTracker) GetConnectionStats() map[string]interface{} {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	var totalMessagesSent, totalMessagesRecv uint64
	var totalBytesSent, totalBytesRecv uint64
	var avgConnectionDuration time.Duration

	now := time.Now()
	connectionDetails := make([]map[string]interface{}, 0, len(ct.connections))

	for _, conn := range ct.connections {
		totalMessagesSent += conn.MessagesSent
		totalMessagesRecv += conn.MessagesRecv
		totalBytesSent += conn.BytesSent
		totalBytesRecv += conn.BytesRecv
		avgConnectionDuration += now.Sub(conn.ConnectedAt)

		// Add connection details
		connectionDetails = append(connectionDetails, map[string]interface{}{
			"id":            conn.ID,
			"remote_addr":   conn.RemoteAddr,
			"duration_sec":  now.Sub(conn.ConnectedAt).Seconds(),
			"messages_sent": conn.MessagesSent,
			"messages_recv": conn.MessagesRecv,
			"bytes_sent":    conn.BytesSent,
			"bytes_recv":    conn.BytesRecv,
			"idle_sec":      now.Sub(conn.LastMessageAt).Seconds(),
		})
	}

	activeCount := len(ct.connections)
	if activeCount > 0 {
		avgConnectionDuration = avgConnectionDuration / time.Duration(activeCount)
	}

	return map[string]interface{}{
		"active":              activeCount,
		"total":               ct.totalConnections,
		"peak":                ct.peakConnections,
		"messages_sent_total": totalMessagesSent,
		"messages_recv_total": totalMessagesRecv,
		"bytes_sent_total":    totalBytesSent,
		"bytes_recv_total":    totalBytesRecv,
		"avg_duration_sec":    avgConnectionDuration.Seconds(),
		"connections":         connectionDetails,
	}
}

// GetSummary returns a summary of connection metrics
func (ct *ConnectionTracker) GetSummary() map[string]interface{} {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	return map[string]interface{}{
		"active": len(ct.connections),
		"total":  ct.totalConnections,
		"peak":   ct.peakConnections,
	}
}