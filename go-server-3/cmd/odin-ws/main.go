package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"odin-ws-server-3/internal/config"
	"odin-ws-server-3/internal/logging"
	"odin-ws-server-3/internal/metrics"
	"odin-ws-server-3/internal/session"
	"odin-ws-server-3/internal/transport"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger, err := logging.NewLogger(cfg.Logging)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync() // nolint:errcheck

	metricsRegistry := metrics.NewRegistry()
	hub := session.NewHub(cfg.WebSocket, metricsRegistry)
	hub.StartBroadcastWorkers()

	transportServer := transport.NewServer(cfg, logger, hub, metricsRegistry)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := transportServer.Start(ctx); err != nil {
		logger.Fatal("transport start failed", zap.Error(err))
	}

	httpErrCh := make(chan error, 1)
	go func() {
		httpErrCh <- runHTTPServer(ctx, cfg, hub, metricsRegistry, logger)
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-httpErrCh:
		if err != nil {
			logger.Error("http server error", zap.Error(err))
		}
		stop()
	}

	transportServer.Stop()
	logger.Info("transport stopped")
}

func runHTTPServer(ctx context.Context, cfg config.Config, hub *session.Hub, metricsRegistry *metrics.Registry, logger *zap.Logger) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"status":    "healthy",
			"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
			"clients":   hub.ClientCount(),
		})
	})

	mux.Handle("/metrics", metricsRegistry.Handler())

	httpServer := &http.Server{
		Addr:         cfg.Metrics.ListenAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("metrics http server starting", zap.String("addr", cfg.Metrics.ListenAddr))
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Warn("metrics http server shutdown error", zap.Error(err))
		}
		return nil
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	}
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
