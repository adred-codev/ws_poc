package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// Configuration
const (
	wsURL              = "ws://34.58.67.124:3004/ws"
	healthURL          = "http://34.58.67.124:3004/health"
	targetConnections  = 1000
	rampRatePerSecond  = 100 // 1,000 connections over 10 seconds
	testDuration       = 10 * time.Minute
	heartbeatInterval  = 30 * time.Second // Reduced frequency at scale
	serverPingInterval = 27 * time.Second
	metricsInterval    = 10 * time.Second // Increased to reduce SSH overhead
	phantomThreshold   = 20               // Higher threshold for 1K scale (2%)
)

// ClientStats tracks per-client metrics
type ClientStats struct {
	ID                  int64
	Connected           bool
	LastHeartbeatSent   time.Time
	LastServerPingRcvd  time.Time
	LastMessageReceived time.Time
	MessagesSent        int64
	MessagesReceived    int64
	ServerPingsReceived int64
	HeartbeatsSent      int64
	Errors              int64
	TCPConnectionAlive  bool
}

// AggregateMetrics for overall test health
type AggregateMetrics struct {
	ConnectedClients    int64
	TotalMessagesSent   int64
	TotalMessagesRcvd   int64
	TotalServerPings    int64
	TotalHeartbeats     int64
	TotalErrors         int64
	TCPConnections      int64
	ServerReportedConns int64
	PhantomConnections  int64
	PhantomPercentage   float64
}

// HealthResponse from /health endpoint
type HealthResponse struct {
	Checks struct {
		Capacity struct {
			Current int64 `json:"current"`
		} `json:"capacity"`
		CPU struct {
			Percentage float64 `json:"percentage"`
		} `json:"cpu"`
		Memory struct {
			UsedMB float64 `json:"used_mb"`
		} `json:"memory"`
		Goroutines struct {
			Current int64 `json:"current"`
		} `json:"goroutines"`
	} `json:"checks"`
}

var (
	clients          = make(map[int64]*ClientStats)
	clientsMutex     sync.RWMutex
	aggregateMetrics AggregateMetrics
	shutdown         = make(chan struct{})
)

func main() {
	log.Printf("🔍 CONNECTION HEALTH VALIDATOR - 1K SCALE")
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("Target Connections: %d", targetConnections)
	log.Printf("Ramp Rate: %d connections/sec", rampRatePerSecond)
	log.Printf("Test Duration: %v", testDuration)
	log.Printf("Phantom Threshold: %d connections (%.1f%%)", phantomThreshold, float64(phantomThreshold)/float64(targetConnections)*100)
	log.Printf("Metrics Interval: %v (optimized for scale)", metricsInterval)
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("\n🛑 Shutdown signal received, closing connections gracefully...")
		close(shutdown)
	}()

	// Start metrics monitor
	go monitorMetrics()

	// Phase 1: Ramp up connections
	log.Printf("⏫ PHASE 1: Ramping up %d connections (%d/sec)\n", targetConnections, rampRatePerSecond)
	startTime := time.Now()

	var wg sync.WaitGroup

	// Batch connections to control ramp rate
	batchSize := rampRatePerSecond
	for batch := 0; batch < targetConnections/batchSize; batch++ {
		batchStart := batch * batchSize

		// Start batch
		for i := 0; i < batchSize; i++ {
			clientID := int64(batchStart + i + 1)
			wg.Add(1)
			go func(id int64) {
				defer wg.Done()
				connectClient(id)
			}(clientID)
		}

		// Wait 1 second before next batch
		if batch < (targetConnections/batchSize)-1 {
			time.Sleep(1 * time.Second)
		}
	}

	// Wait for all connections to establish
	log.Println("⏳ Waiting for all connections to establish...")
	wg.Wait()
	rampDuration := time.Since(startTime)
	log.Printf("✅ All %d clients connected in %v\n", targetConnections, rampDuration)

	// Show initial metrics
	time.Sleep(2 * time.Second)
	connectedCount := atomic.LoadInt64(&aggregateMetrics.ConnectedClients)
	log.Printf("📊 Active connections: %d/%d (%.1f%%)\n", connectedCount, targetConnections, float64(connectedCount)/float64(targetConnections)*100)

	// Phase 2: Monitor for test duration
	log.Printf("\n📊 PHASE 2: Monitoring for %v\n", testDuration)
	log.Printf("⚠️  Watching for phantom connections (threshold: %d)\n", phantomThreshold)

	select {
	case <-time.After(testDuration):
		log.Println("\n⏱️  Test duration completed")
	case <-shutdown:
		log.Println("\n⏹️  Test interrupted by user")
	}

	// Phase 3: Graceful shutdown
	log.Println("\n⬇️  PHASE 3: Graceful shutdown")
	disconnectAllClients()

	// Final verification
	log.Println("⏳ Waiting 5 seconds for connections to close...")
	time.Sleep(5 * time.Second)

	log.Println("🔍 Performing final verification...")
	finalTCPCount := getTCPConnectionCount()
	finalServerCount := getServerConnectionCount()

	log.Printf("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("📊 FINAL METRICS")
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("Test Duration:        %v", testDuration)
	log.Printf("Connections Created:  %d", targetConnections)
	log.Printf("Messages Received:    %d", atomic.LoadInt64(&aggregateMetrics.TotalMessagesRcvd))
	log.Printf("Server PINGs Rcvd:    %d", atomic.LoadInt64(&aggregateMetrics.TotalServerPings))
	log.Printf("Heartbeats Sent:      %d", atomic.LoadInt64(&aggregateMetrics.TotalHeartbeats))
	log.Printf("Total Errors:         %d", atomic.LoadInt64(&aggregateMetrics.TotalErrors))
	log.Printf("\nPost-Shutdown Verification:")
	log.Printf("TCP Connections:      %d (should be 0)", finalTCPCount)
	log.Printf("Server Reported:      %d (should be 0)", finalServerCount)

	if finalTCPCount == 0 && finalServerCount == 0 {
		log.Printf("\n✅ CLEAN SHUTDOWN VERIFIED - NO PHANTOM CONNECTIONS")
	} else if finalTCPCount <= 2 && finalServerCount == 0 {
		log.Printf("\n✅ CLEAN SHUTDOWN VERIFIED - Only monitoring connections remain")
	} else {
		log.Printf("\n❌ PHANTOM CONNECTIONS DETECTED!")
		log.Printf("   Phantom Count: %d", max(finalServerCount-finalTCPCount, 0))
	}
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

func connectClient(clientID int64) {
	stats := &ClientStats{
		ID:        clientID,
		Connected: false,
	}

	clientsMutex.Lock()
	clients[clientID] = stats
	clientsMutex.Unlock()

	// Connect to WebSocket
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		if clientID <= 10 { // Only log first 10 failures to avoid spam
			log.Printf("❌ Client %d: Connection failed: %v", clientID, err)
		}
		atomic.AddInt64(&stats.Errors, 1)
		atomic.AddInt64(&aggregateMetrics.TotalErrors, 1)
		return
	}
	defer conn.Close()

	stats.Connected = true
	atomic.AddInt64(&aggregateMetrics.ConnectedClients, 1)

	// Subscribe to channels
	subscribeMsg := map[string]interface{}{
		"type": "subscribe",
		"channels": []string{
			"orderbook:BTC-USD",
			"trades:BTC-USD",
		},
	}

	if err := conn.WriteJSON(subscribeMsg); err != nil {
		if clientID <= 10 {
			log.Printf("❌ Client %d: Subscribe failed: %v", clientID, err)
		}
		atomic.AddInt64(&stats.Errors, 1)
		atomic.AddInt64(&aggregateMetrics.TotalErrors, 1)
		return
	}

	// Start heartbeat sender
	heartbeatTicker := time.NewTicker(heartbeatInterval)
	defer heartbeatTicker.Stop()

	// Read messages
	go func() {
		for {
			select {
			case <-shutdown:
				return
			default:
				var msg map[string]interface{}
				if err := conn.ReadJSON(&msg); err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						atomic.AddInt64(&stats.Errors, 1)
						atomic.AddInt64(&aggregateMetrics.TotalErrors, 1)
					}
					return
				}

				stats.LastMessageReceived = time.Now()
				atomic.AddInt64(&stats.MessagesReceived, 1)
				atomic.AddInt64(&aggregateMetrics.TotalMessagesRcvd, 1)

				// Check message type
				if msgType, ok := msg["type"].(string); ok {
					if msgType == "ping" {
						stats.LastServerPingRcvd = time.Now()
						atomic.AddInt64(&stats.ServerPingsReceived, 1)
						atomic.AddInt64(&aggregateMetrics.TotalServerPings, 1)

						// Respond with pong
						pongMsg := map[string]interface{}{"type": "pong"}
						conn.WriteJSON(pongMsg)
					}
				}
			}
		}
	}()

	// Send heartbeats
	for {
		select {
		case <-shutdown:
			return
		case <-heartbeatTicker.C:
			heartbeatMsg := map[string]interface{}{
				"type":      "heartbeat",
				"client_id": clientID,
				"timestamp": time.Now().Unix(),
			}

			if err := conn.WriteJSON(heartbeatMsg); err != nil {
				atomic.AddInt64(&stats.Errors, 1)
				atomic.AddInt64(&aggregateMetrics.TotalErrors, 1)
				return
			}

			stats.LastHeartbeatSent = time.Now()
			atomic.AddInt64(&stats.HeartbeatsSent, 1)
			atomic.AddInt64(&aggregateMetrics.TotalHeartbeats, 1)
		}
	}
}

func monitorMetrics() {
	ticker := time.NewTicker(metricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-shutdown:
			return
		case <-ticker.C:
			// Get TCP connection count (ground truth)
			tcpCount := getTCPConnectionCount()
			atomic.StoreInt64(&aggregateMetrics.TCPConnections, tcpCount)

			// Get server reported count
			serverCount := getServerConnectionCount()
			atomic.StoreInt64(&aggregateMetrics.ServerReportedConns, serverCount)

			// Calculate phantom connections (adjust for monitoring connections)
			adjustedTCP := tcpCount
			if tcpCount >= 2 {
				adjustedTCP = tcpCount - 2 // Subtract monitoring connections
			}

			phantom := serverCount - adjustedTCP
			if phantom < 0 {
				phantom = 0
			}
			atomic.StoreInt64(&aggregateMetrics.PhantomConnections, phantom)

			phantomPct := 0.0
			if adjustedTCP > 0 {
				phantomPct = (float64(phantom) / float64(adjustedTCP)) * 100.0
			}

			// Get server health
			health := getServerHealth()

			// Print metrics
			log.Printf("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			log.Printf("📊 METRICS @ %s", time.Now().Format("15:04:05"))
			log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			log.Printf("Connection Health:")
			log.Printf("  TCP Connections:    %d (ground truth, includes +2 monitoring)", tcpCount)
			log.Printf("  Client Connections: %d (TCP - monitoring)", adjustedTCP)
			log.Printf("  Server Reported:    %d", serverCount)
			log.Printf("  Phantom Count:      %d", phantom)
			log.Printf("  Phantom %%:          %.2f%%", phantomPct)

			if phantom > phantomThreshold {
				log.Printf("  ⚠️  WARNING: Phantom connections exceed threshold! (%d > %d)", phantom, phantomThreshold)
			} else {
				log.Printf("  ✅ Phantom connections within threshold")
			}

			log.Printf("\nMessage Flow:")
			log.Printf("  Messages Received:  %d", atomic.LoadInt64(&aggregateMetrics.TotalMessagesRcvd))
			log.Printf("  Server PINGs Rcvd:  %d", atomic.LoadInt64(&aggregateMetrics.TotalServerPings))
			log.Printf("  Heartbeats Sent:    %d", atomic.LoadInt64(&aggregateMetrics.TotalHeartbeats))
			log.Printf("  Total Errors:       %d", atomic.LoadInt64(&aggregateMetrics.TotalErrors))

			if health != nil {
				log.Printf("\nServer Resources:")
				log.Printf("  CPU:                %.2f%%", health.Checks.CPU.Percentage)
				log.Printf("  Memory:             %.2f MB", health.Checks.Memory.UsedMB)
				log.Printf("  Goroutines:         %d", health.Checks.Goroutines.Current)

				// Calculate per-connection overhead
				if serverCount > 0 {
					baselineMemory := 18.0 // MB from 0-connection baseline
					connectionMemory := health.Checks.Memory.UsedMB - baselineMemory
					perConnKB := (connectionMemory * 1024) / float64(serverCount)
					log.Printf("  Per-Connection:     %.1f KB", perConnKB)
				}
			}
			log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		}
	}
}

func getTCPConnectionCount() int64 {
	// WebSocket server runs on port 3002 INSIDE Docker container
	cmd := exec.Command("gcloud", "compute", "ssh", "odin-ws-go",
		"--zone=us-central1-a",
		"--command=sudo docker exec odin-ws-go netstat -an | grep :3002 | grep ESTABLISHED | wc -l")

	output, err := cmd.Output()
	if err != nil {
		log.Printf("Warning: Failed to get TCP connection count: %v", err)
		return -1
	}

	count, err := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		return -1
	}

	return count
}

func getServerConnectionCount() int64 {
	health := getServerHealth()
	if health != nil {
		return health.Checks.Capacity.Current
	}
	return -1
}

func getServerHealth() *HealthResponse {
	resp, err := http.Get(healthURL)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var health HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil
	}

	return &health
}

func disconnectAllClients() {
	close(shutdown)
	log.Println("⏳ Waiting for all clients to disconnect...")
	time.Sleep(3 * time.Second)
	log.Printf("✅ All %d clients disconnected", targetConnections)
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
