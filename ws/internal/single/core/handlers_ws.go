package core

import (
	"net/http"
	"sync/atomic"
	"time"

	"github.com/adred-codev/ws_poc/internal/single/monitoring"
	"github.com/gobwas/ws"
)

// WebSocket upgrade handler
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Reject new connections during graceful shutdown
	if atomic.LoadInt32(&s.shuttingDown) == 1 {
		http.Error(w, "Server is shutting down", http.StatusServiceUnavailable)
		return
	}

	// ResourceGuard admission control - static limits with safety checks
	shouldAccept, reason := s.resourceGuard.ShouldAcceptConnection()
	if !shouldAccept {
		currentConnections := atomic.LoadInt64(&s.stats.CurrentConnections)
		// Removed audit logger call - rejections tracked via Prometheus metrics
		// s.auditLogger.Warning("ConnectionRejected", reason, map[string]any{...})
		s.logger.Debug().
			Int64("current_connections", currentConnections).
			Int("max_connections", s.config.MaxConnections).
			Str("reason", reason).
			Msg("Connection rejected by ResourceGuard")
		monitoring.ConnectionsFailed.Inc()
		http.Error(w, "Server overloaded", http.StatusServiceUnavailable)
		return
	}

	// Try to acquire connection slot (non-blocking with timeout)
	select {
	case s.connectionsSem <- struct{}{}:
		// Slot acquired
	case <-time.After(5 * time.Second):
		// Server at capacity, reject with 503
		s.auditLogger.Warning("ServerAtCapacity", "Connection rejected - server at maximum capacity", map[string]any{
			"currentConnections": atomic.LoadInt64(&s.stats.CurrentConnections),
			"maxConnections":     s.config.MaxConnections,
		})
		monitoring.ConnectionsFailed.Inc()
		http.Error(w, "Server at capacity", http.StatusServiceUnavailable)
		return
	}

	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		<-s.connectionsSem // Release slot
		s.auditLogger.Error("WebSocketUpgradeFailed", "Failed to upgrade HTTP connection to WebSocket", map[string]any{
			"error":      err.Error(),
			"remoteAddr": r.RemoteAddr,
		})
		monitoring.ConnectionsFailed.Inc()
		s.logger.Error().
			Err(err).
			Msg("Failed to upgrade connection")
		return
	}

	client := s.connections.Get()
	client.conn = conn
	client.server = s
	client.id = atomic.AddInt64(&s.clientCount, 1)

	s.clients.Store(client, true)
	atomic.AddInt64(&s.stats.TotalConnections, 1)
	atomic.AddInt64(&s.stats.CurrentConnections, 1)

	// Update Prometheus metrics
	monitoring.UpdateConnectionMetrics(s)

	go s.writePump(client)
	go s.readPump(client)
}

// disconnectClient handles client disconnect with proper instrumentation
// Centralizes all disconnect logic to ensure consistent metrics and logging
