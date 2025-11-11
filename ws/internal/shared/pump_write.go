package shared

import (
	"bufio"
	"sync/atomic"
	"time"

	"github.com/adred-codev/ws_poc/internal/shared/monitoring"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// writePump batches messages and writes them to the WebSocket connection.
// This is a hot path and has been optimized to reduce system calls.
func (s *Server) writePump(c *Client) {
	// Use a buffered writer to batch writes and reduce syscalls.
	writer := bufio.NewWriter(c.conn)
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
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
				s.logger.Debug().Int64("client_id", c.id).Msg("Send channel closed")
				wsutil.WriteServerMessage(c.conn, ws.OpClose, []byte{})
				return
			}

			// Set a deadline for the write operation.
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))

			// Write the first message
			err := wsutil.WriteServerMessage(writer, ws.OpText, message)
			if err != nil {
				s.logger.Debug().Err(err).Int64("client_id", c.id).Msg("Failed to write message")
				return
			}
			atomic.AddInt64(&s.stats.MessagesSent, 1)
			atomic.AddInt64(&s.stats.BytesSent, int64(len(message)))
			monitoring.UpdateMessageMetrics(1, 0)
			monitoring.UpdateBytesMetrics(int64(len(message)), 0)

			// Batch additional messages from the channel.
			n := len(c.send)
			for i := 0; i < n; i++ {
				message = <-c.send
				err := wsutil.WriteServerMessage(writer, ws.OpText, message)
				if err != nil {
					s.logger.Debug().Err(err).Int64("client_id", c.id).Msg("Failed to write message")
					return
				}
				atomic.AddInt64(&s.stats.MessagesSent, 1)
				atomic.AddInt64(&s.stats.BytesSent, int64(len(message)))
				monitoring.UpdateMessageMetrics(1, 0)
				monitoring.UpdateBytesMetrics(int64(len(message)), 0)
			}

			// Flush the buffer to send all batched messages.
			if err := writer.Flush(); err != nil {
				s.logger.Debug().Err(err).Int64("client_id", c.id).Msg("Failed to flush writer")
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := wsutil.WriteServerMessage(c.conn, ws.OpPing, nil); err != nil {
				s.logger.Debug().Err(err).Int64("client_id", c.id).Msg("Failed to send ping")
				return
			}
		}
	}
}



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
