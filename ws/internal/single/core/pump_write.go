package core

import (
	"sync/atomic"
	"time"

	"github.com/adred-codev/ws_poc/internal/single/monitoring"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// writePump writes messages to the WebSocket connection (HOT PATH - 97% CPU)
func (s *Server) writePump(c *Client) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		// Use sync.Once to ensure connection is only closed once
		// Prevents race condition with readPump also trying to close
		c.closeOnce.Do(func() {
			if c.conn != nil {
				c.conn.Close()
			}
		})
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				// The server closed the channel.
				s.logger.Debug().
					Int64("client_id", c.id).
					Str("reason", "send_channel_closed").
					Msg("Client send channel closed, disconnecting")
				if c.conn != nil {
					wsutil.WriteServerMessage(c.conn, ws.OpClose, []byte{})
				}
				return
			}

			if c.conn == nil {
				s.logger.Warn().
					Int64("client_id", c.id).
					Str("reason", "connection_nil").
					Msg("Client connection is nil in writePump")
				return
			}

			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			err := wsutil.WriteServerMessage(c.conn, ws.OpText, message)
			if err != nil {
				s.logger.Debug().
					Int64("client_id", c.id).
					Err(err).
					Str("reason", "write_error").
					Int("message_size", len(message)).
					Msg("Failed to write message to client")
				return
			}
			atomic.AddInt64(&s.stats.MessagesSent, 1)
			atomic.AddInt64(&s.stats.BytesSent, int64(len(message)))
			monitoring.UpdateMessageMetrics(1, 0)
			monitoring.UpdateBytesMetrics(int64(len(message)), 0)

		case <-ticker.C:
			if c.conn == nil {
				s.logger.Warn().
					Int64("client_id", c.id).
					Str("reason", "connection_nil_ping").
					Msg("Client connection is nil during ping")
				return
			}
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := wsutil.WriteServerMessage(c.conn, ws.OpPing, nil); err != nil {
				s.logger.Debug().
					Int64("client_id", c.id).
					Err(err).
					Str("reason", "ping_write_error").
					Msg("Failed to send ping to client")
				return
			}
		}
	}
}

// extractChannel extracts the hierarchical channel identifier from a Kafka topic subject
// Subject format: "odin.token.BTC.trade" → extracts "BTC.trade"
//
// Subject structure:
// - Part 0: Namespace ("odin")
// - Part 1: Type ("token")
// - Part 2: Symbol ("BTC", "ETH", "SOL")
// - Part 3: Event Type (REQUIRED) ("trade", "liquidity", "metadata", "social", "favorites", "creation", "analytics", "balances")
//
// Event Types (8 channels per symbol):
// 1. "trade"      - Real-time trading (price, volume) - User-initiated, high-frequency
// 2. "liquidity"  - Liquidity operations (add/remove) - User-initiated
// 3. "metadata"   - Token metadata updates - Manual, infrequent
// 4. "social"     - Comments, reactions - User-initiated
// 5. "favorites"  - User bookmarks - User-initiated
// 6. "creation"   - Token launches - User-initiated
// 7. "analytics"  - Background metrics (holder counts, TVL) - Scheduler-driven, low-frequency
// 8. "balances"   - Wallet balance changes - User-initiated
//
// Returns: "BTC.trade" for "odin.token.BTC.trade" or empty string if invalid format
//
// Performance Impact:
// - Clients subscribe to specific event types: "BTC.trade" instead of all BTC events
// - 8x reduction in unnecessary messages per subscribed symbol
// - Example: Trading client subscribes to ["BTC.trade", "ETH.trade"] only
//
// Future Enhancement (Phase 2):
// - Move price deltas from scheduler to real-time trade events (<100ms latency)
// - See: /Volumes/Dev/Codev/Toniq/ws_poc/docs/production/ODIN_API_IMPROVEMENTS.md
