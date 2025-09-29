package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"odin-ws-server/internal/auth"
	"odin-ws-server/internal/metrics"
	"odin-ws-server/internal/types"
	natsClient "odin-ws-server/pkg/nats"
	wsClient "odin-ws-server/pkg/websocket"
)

type Server struct {
	config           *types.Config
	httpServer       *http.Server
	hub              *wsClient.Hub
	nats             *natsClient.Client
	enhancedMetrics  *metrics.EnhancedMetrics
	jwtManager       *auth.JWTManager
	logger           *log.Logger
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
}

func NewServer(config *types.Config) (*Server, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create logger
	logger := log.New(os.Stdout, "[ODIN-WS] ", log.LstdFlags|log.Lshortfile)

	// Create enhanced metrics (no Prometheus)
	enhancedMetricsInstance := metrics.NewEnhancedMetrics()

	// Create JWT manager
	jwtManager := auth.NewJWTManager(
		config.Auth.JWTSecret,
		time.Duration(config.Auth.TokenExpiration)*time.Second,
	)

	// Create WebSocket hub
	hub := wsClient.NewHub(enhancedMetricsInstance, logger)

	// Set enhanced metrics on hub after it's created
	hub.SetEnhancedMetrics(enhancedMetricsInstance)

	// Create NATS client
	natsConfig := natsClient.Config{
		URL:               config.NATS.URL,
		MaxReconnects:     config.NATS.MaxReconnects,
		ReconnectWait:     time.Duration(config.NATS.ReconnectWait) * time.Millisecond,
		ReconnectJitter:   time.Duration(config.NATS.ReconnectJitter) * time.Millisecond,
		MaxPingsOut:       config.NATS.MaxPingsOut,
		PingInterval:      time.Duration(config.NATS.PingInterval) * time.Millisecond,
	}

	natsClientInstance, err := natsClient.NewClient(natsConfig, enhancedMetricsInstance, logger)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create NATS client: %w", err)
	}

	server := &Server{
		config:          config,
		hub:             hub,
		nats:            natsClientInstance,
		enhancedMetrics: enhancedMetricsInstance,
		jwtManager:      jwtManager,
		logger:          logger,
		ctx:             ctx,
		cancel:          cancel,
	}

	// Create HTTP server
	server.setupHTTPServer()

	return server, nil
}

func (s *Server) setupHTTPServer() {
	mux := http.NewServeMux()

	// WebSocket endpoint
	mux.HandleFunc("/ws", s.handleWebSocket)

	// Health check endpoint
	mux.HandleFunc("/health", s.handleHealth)

	// Stats endpoint (legacy)
	mux.HandleFunc("/stats", s.handleStats)

	// Enhanced metrics endpoint
	mux.HandleFunc("/metrics/enhanced", s.handleEnhancedMetrics)

	// System metrics endpoint
	mux.HandleFunc("/metrics/system", s.handleSystemMetrics)

	// Dashboard endpoint removed - using React client instead
	// mux.HandleFunc("/dashboard", s.handleDashboard)

	// Generate test token endpoint (development only)
	mux.HandleFunc("/auth/token", s.handleGenerateToken)

	// CORS middleware
	corsHandler := s.corsMiddleware(mux)

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port),
		Handler:      corsHandler,
		ReadTimeout:  time.Duration(s.config.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(s.config.Server.WriteTimeout) * time.Second,
	}
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Authenticate if required
	if s.config.Auth.RequireAuth {
		claims, err := s.jwtManager.WebSocketAuth(r)
		if err != nil {
			s.logger.Printf("WebSocket authentication failed: %v", err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			// Record error in enhanced metrics
			return
		}
		s.logger.Printf("WebSocket authenticated user: %s", claims.UserID)
	}

	// Upgrade to WebSocket
	wsClient.ServeWS(s.hub, s.enhancedMetrics, s.logger, w, r)

	// Record latency in enhanced metrics
	// s.enhancedMetrics.RecordMessageLatency(time.Since(start))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
		"uptime":    s.enhancedMetrics.GetSimpleStats(),
		"services": map[string]interface{}{
			"websocket": map[string]interface{}{
				"status": "healthy",
				"clients": s.hub.GetClientCount(),
			},
			"nats": map[string]interface{}{
				"status":    s.getNATSStatus(),
				"connected": s.nats.IsConnected(),
			},
		},
		"system": map[string]interface{}{
			"goroutines": runtime.NumGoroutine(),
			"memory":     getMemoryStats(),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	// Use enhanced metrics for more accurate data
	stats := s.enhancedMetrics.GetSimpleStats()

	// Add additional legacy data
	stats["nats"] = s.nats.Stats()
	stats["hub"] = s.hub.GetStats()
	stats["uptime_seconds"] = time.Since(time.Now()).Seconds() // Will be calculated in enhanced metrics

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleEnhancedMetrics(w http.ResponseWriter, r *http.Request) {
	stats := s.enhancedMetrics.GetAccurateStats()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleSystemMetrics(w http.ResponseWriter, r *http.Request) {
	systemStats := map[string]interface{}{
		"timestamp": time.Now().Unix(),
		"system":    s.enhancedMetrics.GetAccurateStats()["system"],
		"performance": s.enhancedMetrics.GetAccurateStats()["performance"],
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(systemStats)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	// Read the dashboard HTML file
	// Try container path first, then local development path
	dashboardPaths := []string{
		"static/dashboard.html",         // Docker container path
		"go-server/static/dashboard.html", // Local development path
	}

	var content []byte
	var err error
	for _, path := range dashboardPaths {
		content, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}

	if err != nil {
		s.logger.Printf("Error reading dashboard file: %v", err)
		http.Error(w, "Dashboard not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write(content)
}

func (s *Server) handleGenerateToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token, err := s.jwtManager.GenerateTestToken()
	if err != nil {
		s.logger.Printf("Error generating test token: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	response := map[string]string{
		"token": token,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Requested-With")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) Start() error {
	s.logger.Printf("Starting Odin WebSocket Server...")

	// Start the hub
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.hub.Run()
	}()

	// Setup NATS subscriptions
	if err := s.setupNATSSubscriptions(); err != nil {
		return fmt.Errorf("failed to setup NATS subscriptions: %w", err)
	}

	// Start system metrics collection
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.collectSystemMetrics()
	}()

	// Start enhanced metrics collection
	s.enhancedMetrics.StartCollection()

	// Start HTTP server
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.logger.Printf("HTTP server listening on %s", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Printf("HTTP server error: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	s.waitForShutdown()

	return nil
}

func (s *Server) setupNATSSubscriptions() error {
	// Subscribe to all price updates
	if err := s.nats.Subscribe("odin.token.*.price", s.handlePriceUpdate); err != nil {
		return fmt.Errorf("failed to subscribe to price updates: %w", err)
	}

	// Subscribe to batch updates
	if err := s.nats.Subscribe(natsClient.SubjectBuilder.BatchUpdate(), s.handleBatchUpdate); err != nil {
		return fmt.Errorf("failed to subscribe to batch updates: %w", err)
	}

	// Subscribe to trade executions
	if err := s.nats.Subscribe("odin.trades.*", s.handleTradeUpdate); err != nil {
		return fmt.Errorf("failed to subscribe to trade updates: %w", err)
	}

	// Subscribe to market statistics
	if err := s.nats.Subscribe(natsClient.SubjectBuilder.MarketStats(), s.handleMarketStats); err != nil {
		return fmt.Errorf("failed to subscribe to market stats: %w", err)
	}

	s.logger.Printf("NATS subscriptions established")
	return nil
}

func (s *Server) handlePriceUpdate(data []byte) {
	s.hub.BroadcastMessage(data)
}

func (s *Server) handleBatchUpdate(data []byte) {
	s.hub.BroadcastMessage(data)
}

func (s *Server) handleTradeUpdate(data []byte) {
	s.hub.BroadcastMessage(data)
}

func (s *Server) handleMarketStats(data []byte) {
	s.hub.BroadcastMessage(data)
}

func (s *Server) collectSystemMetrics() {
	ticker := time.NewTicker(time.Duration(s.config.Metrics.UpdateInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.updateSystemMetrics()
		}
	}
}

func (s *Server) updateSystemMetrics() {
	// Enhanced metrics now handle all system updates automatically
	// This method is kept for compatibility but the real work is done
	// in enhancedMetrics.updateAllMetrics()
}

func (s *Server) getNATSStatus() string {
	switch s.nats.Status() {
	case 1: // CONNECTED
		return "connected"
	case 0: // DISCONNECTED
		return "disconnected"
	case 2: // CONNECTING
		return "connecting"
	case 3: // RECONNECTING
		return "reconnecting"
	case 4: // CLOSED
		return "closed"
	default:
		return "unknown"
	}
}

func (s *Server) waitForShutdown() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	s.logger.Printf("Received signal %v, initiating graceful shutdown...", sig)

	s.Shutdown()
}

func (s *Server) Shutdown() {
	s.logger.Printf("Shutting down server...")

	// Cancel context to stop all goroutines
	s.cancel()

	// Create a context with timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown HTTP server
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Printf("HTTP server shutdown error: %v", err)
	}

	// Close NATS connection
	if err := s.nats.Close(); err != nil {
		s.logger.Printf("NATS close error: %v", err)
	}

	// Shutdown hub
	s.hub.Shutdown()

	// Wait for all goroutines to finish
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Printf("Server shutdown complete")
	case <-ctx.Done():
		s.logger.Printf("Server shutdown timeout")
	}
}

// Utility functions
func getMemoryStats() map[string]interface{} {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return map[string]interface{}{
		"alloc":        m.Alloc,
		"total_alloc":  m.TotalAlloc,
		"sys":          m.Sys,
		"heap_alloc":   m.HeapAlloc,
		"heap_sys":     m.HeapSys,
		"heap_idle":    m.HeapIdle,
		"heap_inuse":   m.HeapInuse,
		"stack_inuse":  m.StackInuse,
		"stack_sys":    m.StackSys,
		"num_gc":       m.NumGC,
		"gc_cpu_fraction": m.GCCPUFraction,
	}
}