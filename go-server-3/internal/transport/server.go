package transport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"go.uber.org/zap"

	"odin-ws-server-3/internal/config"
	"odin-ws-server-3/internal/metrics"
	"odin-ws-server-3/internal/session"
)

// Server handles TCP listening and WebSocket upgrades using gobwas/ws.
type Server struct {
	cfg      config.Config
	logger   *zap.Logger
	hub      *session.Hub
	metrics  *metrics.Registry
	listener net.Listener
	wg       sync.WaitGroup
}

func NewServer(cfg config.Config, logger *zap.Logger, hub *session.Hub, metricsRegistry *metrics.Registry) *Server {
	return &Server{cfg: cfg, logger: logger, hub: hub, metrics: metricsRegistry}
}

func (s *Server) Start(ctx context.Context) error {
	if s.listener != nil {
		return errors.New("transport already started")
	}

	addr := fmt.Sprintf("%s:%d", s.cfg.Server.Host, s.cfg.Server.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	s.listener = ln
	s.logger.Info("transport listening", zap.String("addr", addr))

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.acceptLoop(ctx)
	}()

	return nil
}

func (s *Server) Stop() {
	if s.listener != nil {
		_ = s.listener.Close()
	}
	s.wg.Wait()
}

func (s *Server) acceptLoop(ctx context.Context) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			if ctx.Err() != nil {
				return
			}
			s.logger.Error("accept error", zap.Error(err))
			return
		}

		if s.metrics != nil {
			s.metrics.Connections.ActiveConnections.Inc()
		}

		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			s.handleConnection(ctx, c)
			if s.metrics != nil {
				s.metrics.Connections.ActiveConnections.Dec()
			}
		}(conn)
	}
}

func (s *Server) handleConnection(parent context.Context, conn net.Conn) {
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		s.logger.Debug("set deadline", zap.Error(err))
	}

	if _, err := ws.Upgrade(conn); err != nil {
		if s.metrics != nil {
			s.metrics.Messages.AcceptErrors.Inc()
		}
		s.logger.Debug("upgrade failed", zap.Error(err))
		return
	}

	_ = conn.SetDeadline(time.Time{})

	registration := s.hub.Register(conn)
	if registration == nil {
		return
	}
	defer s.hub.Unregister(registration)

	connCtx, cancel := context.WithCancel(parent)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.writeLoop(connCtx, registration, conn)
	}()

	s.readLoop(connCtx, conn)
	cancel()
	<-done
}

func (s *Server) readLoop(ctx context.Context, conn net.Conn) {
	reader := wsutil.NewReader(conn, ws.StateServerSide)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		head, err := reader.NextFrame()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				s.logger.Debug("read frame error", zap.Error(err))
			}
			return
		}

		switch head.OpCode {
		case ws.OpClose:
			_ = wsutil.WriteServerMessage(conn, ws.OpClose, nil)
			return
		case ws.OpPing:
			if err := wsutil.WriteServerMessage(conn, ws.OpPong, nil); err != nil {
				s.logger.Debug("write pong error", zap.Error(err))
				return
			}
		case ws.OpText, ws.OpBinary:
			payload := make([]byte, head.Length)
			if _, err := io.ReadFull(reader, payload); err != nil {
				s.logger.Debug("read message data error", zap.Error(err))
				return
			}
			s.hub.Broadcast(payload)
		default:
			if _, err := io.CopyN(io.Discard, reader, int64(head.Length)); err != nil {
				s.logger.Debug("drain frame data error", zap.Error(err))
				return
			}
		}
	}
}

func (s *Server) writeLoop(ctx context.Context, connState *session.Connection, conn net.Conn) {
	for {
		select {
		case <-ctx.Done():
			return
		case payload, ok := <-connState.SendQueue:
			if !ok {
				return
			}
			if err := wsutil.WriteServerMessage(conn, ws.OpBinary, payload); err != nil {
				s.logger.Debug("write message error", zap.Error(err))
				return
			}
		}
	}
}
