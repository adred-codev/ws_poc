#!/usr/bin/env node

/**
 * Sustained Load Test for WebSocket Server
 *
 * Purpose: Test server stability under sustained load
 * - Gradual ramp-up (avoid burst rejections)
 * - Hold connections stable (no churn)
 * - Active message flow throughout
 * - Monitor metrics over time
 */

const WebSocket = require('ws');
const http = require('http');

// ============================================================================
// CONFIGURATION
// ============================================================================
//
// IMPORTANT: Test configuration should match WebSocket server configuration!
//
// The server's resource limits are configured in docker-compose.yml:
//   - WS_MAX_CONNECTIONS: 500 (hard connection limit)
//   - WS_MAX_NATS_RATE: 20 messages/sec (rate limit)
//   - WS_CPU_REJECT_THRESHOLD: 75% (emergency brake)
//
// TEST MODES:
//
// 1. CAPACITY TEST (recommended default):
//    - Set TARGET_CONNECTIONS = server's WS_MAX_CONNECTIONS (500)
//    - Set RAMP_RATE to gradual value (50-100 conn/sec)
//    - Expected: All connections succeed, server stable, no rejections
//    - Purpose: Verify server handles maximum configured load
//    - Example: TARGET_CONNECTIONS=500 RAMP_RATE=50 npm run test:sustained
//
// 2. STRESS/OVERLOAD TEST (intentional overload):
//    - Set TARGET_CONNECTIONS > server's WS_MAX_CONNECTIONS (e.g., 1500-2000)
//    - Set RAMP_RATE higher (100-200 conn/sec)
//    - Expected: Rejections once limit hit, server stays stable, no crash
//    - Purpose: Verify server correctly rejects excess load without crashing
//    - Example: TARGET_CONNECTIONS=1500 RAMP_RATE=100 npm run test:sustained
//
// 3. BURST/SPIKE TEST (rapid connection attempts):
//    - Set TARGET_CONNECTIONS to server limit (500)
//    - Set RAMP_RATE very high (500-1000 conn/sec)
//    - Expected: Some rejections during burst, server recovers, final count = limit
//    - Purpose: Verify server handles traffic spikes gracefully
//    - Example: TARGET_CONNECTIONS=500 RAMP_RATE=500 npm run test:sustained
//
// ============================================================================

const CONFIG = {
  // Server settings
  WS_URL: process.env.WS_URL || 'ws://localhost:3004/ws',
  HEALTH_URL: process.env.HEALTH_URL || 'http://localhost:3004/health',

  // Target connections
  // DEFAULT: Matches server's WS_MAX_CONNECTIONS for capacity testing
  // Override with TARGET_CONNECTIONS env var for stress testing
  TARGET_CONNECTIONS: parseInt(process.env.TARGET_CONNECTIONS) || 500,

  // Ramp-up settings (gradual increase)
  // DEFAULT: Moderate ramp for smooth capacity testing
  // Override with RAMP_RATE env var for burst/stress testing
  RAMP_RATE: parseInt(process.env.RAMP_RATE) || 50, // connections per second

  // Sustain duration (after reaching target)
  // DEFAULT: 30 minutes to verify long-term stability
  // Override with DURATION env var (in seconds) for shorter tests
  SUSTAIN_DURATION_MS: parseInt(process.env.DURATION) * 1000 || 1800000, // Default 30 min

  // Reporting interval
  REPORT_INTERVAL_MS: 10000, // Report every 10 seconds

  // Health check interval
  HEALTH_CHECK_INTERVAL_MS: 5000, // Check every 5 seconds
};

// ============================================================================
// STATE MANAGEMENT
// ============================================================================

const state = {
  // Connection tracking
  activeConnections: new Map(),
  totalConnectionsCreated: 0,
  failedConnections: 0,

  // Metrics
  messagesReceived: 0,
  errors: 0,
  connectionErrors: {},

  // Health monitoring
  lastHealthCheck: null,

  // Timing
  startTime: Date.now(),
  rampStartTime: Date.now(),
  sustainStartTime: null,
  phase: 'ramping', // 'ramping', 'sustaining', 'completed'
};

// ============================================================================
// CONNECTION CLASS
// ============================================================================

class LoadTestConnection {
  constructor(id) {
    this.id = id;
    this.ws = null;
    this.messagesReceived = 0;
    this.connected = false;
    this.connectTime = null;
  }

  connect() {
    return new Promise((resolve) => {
      try {
        this.ws = new WebSocket(CONFIG.WS_URL);

        this.ws.on('open', () => {
          this.connected = true;
          this.connectTime = Date.now();
          state.activeConnections.set(this.id, this);
          this.startHeartbeat(); // Start sending heartbeats to prevent timeout
          resolve(true);
        });

        this.ws.on('message', (data) => {
          this.onMessage(data);
        });

        this.ws.on('error', (error) => {
          this.onError(error);
        });

        this.ws.on('close', () => {
          this.onClose();
        });

        // Handle ping/pong to prevent timeout
        this.ws.on('ping', () => {
          if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.ws.pong();
          }
        });

        // Connection timeout (5 seconds)
        setTimeout(() => {
          if (!this.connected) {
            this.ws.terminate();
            resolve(false);
          }
        }, 5000);

      } catch (error) {
        this.onError(error);
        resolve(false);
      }
    });
  }

  onMessage(data) {
    this.messagesReceived++;
    state.messagesReceived++;

    // Basic validation
    try {
      const message = JSON.parse(data.toString());
      if (!message.type) {
        state.errors++;
      }
    } catch (error) {
      state.errors++;
    }
  }

  onError(error) {
    const errorCode = error.code || 'UNKNOWN';
    state.connectionErrors[errorCode] = (state.connectionErrors[errorCode] || 0) + 1;
  }

  onClose() {
    this.connected = false;
    state.activeConnections.delete(this.id);
    if (this.heartbeatInterval) {
      clearInterval(this.heartbeatInterval);
    }
  }

  startHeartbeat() {
    // Send heartbeat every 30 seconds to prevent server read timeout (60s)
    this.heartbeatInterval = setInterval(() => {
      if (this.ws && this.ws.readyState === WebSocket.OPEN) {
        try {
          this.ws.send(JSON.stringify({ type: 'heartbeat' }));
        } catch (error) {
          // Ignore send errors
        }
      }
    }, 30000); // 30 seconds (half of server's 60s timeout)
  }

  close() {
    if (this.heartbeatInterval) {
      clearInterval(this.heartbeatInterval);
    }
    if (this.ws && this.connected) {
      this.ws.close(1000, 'Test completed');
    }
  }
}

// ============================================================================
// HEALTH MONITORING
// ============================================================================

async function checkServerHealth() {
  try {
    const response = await fetch(CONFIG.HEALTH_URL);
    const data = await response.json();
    state.lastHealthCheck = data;
    return data;
  } catch (error) {
    log(`❌ Health check failed: ${error.message}`, 'ERROR');
    return null;
  }
}

// ============================================================================
// CONNECTION MANAGEMENT
// ============================================================================

async function rampUpConnections() {
  return new Promise((resolve) => {
    const connectionsPerBatch = Math.ceil(CONFIG.RAMP_RATE / 10); // Spread over 10 batches per second
    const batchInterval = 1000 / 10; // 100ms between batches

    log(`🚀 Starting ramp-up: ${CONFIG.TARGET_CONNECTIONS} connections at ${CONFIG.RAMP_RATE}/sec`, 'INFO');

    let connectionId = 0;

    const rampInterval = setInterval(async () => {
      // Check if we've reached target
      if (state.totalConnectionsCreated >= CONFIG.TARGET_CONNECTIONS) {
        clearInterval(rampInterval);
        state.phase = 'sustaining';
        state.sustainStartTime = Date.now();
        log(`✅ Ramp-up complete! ${state.activeConnections.size} connections established`, 'INFO');
        log(`🔒 Sustaining load for ${CONFIG.SUSTAIN_DURATION_MS / 1000}s...`, 'INFO');
        resolve();
        return;
      }

      // Create batch of connections
      const promises = [];
      for (let i = 0; i < connectionsPerBatch && state.totalConnectionsCreated < CONFIG.TARGET_CONNECTIONS; i++) {
        const conn = new LoadTestConnection(connectionId++);
        state.totalConnectionsCreated++;
        promises.push(conn.connect());
      }

      const results = await Promise.all(promises);
      const failed = results.filter(r => !r).length;
      state.failedConnections += failed;

    }, batchInterval);
  });
}

function closeAllConnections() {
  log(`🔄 Closing ${state.activeConnections.size} connections...`, 'INFO');

  for (const conn of state.activeConnections.values()) {
    conn.close();
  }

  state.activeConnections.clear();
}

// ============================================================================
// REPORTING
// ============================================================================

function log(message, level = 'INFO') {
  const elapsed = ((Date.now() - state.startTime) / 1000).toFixed(1);
  const timestamp = new Date().toISOString();
  console.log(`[${timestamp}] [${level}] [+${elapsed}s] ${message}`);
}

function printReport() {
  const elapsed = Math.floor((Date.now() - state.startTime) / 1000);
  const health = state.lastHealthCheck;

  const cpuUsage = health?.checks?.cpu?.percentage?.toFixed(1) || 'N/A';
  const memUsage = health?.checks?.memory?.percentage?.toFixed(1) || 'N/A';
  const serverConns = health?.checks?.capacity?.current || 'N/A';

  const elapsedSustain = state.sustainStartTime
    ? Math.floor((Date.now() - state.sustainStartTime) / 1000)
    : 0;

  const remainingSustain = state.sustainStartTime
    ? Math.max(0, Math.floor(CONFIG.SUSTAIN_DURATION_MS / 1000) - elapsedSustain)
    : Math.floor(CONFIG.SUSTAIN_DURATION_MS / 1000);

  const msgRate = state.messagesReceived / elapsed;
  const successRate = ((state.totalConnectionsCreated - state.failedConnections) / state.totalConnectionsCreated * 100).toFixed(1);

  console.log('\n' + '='.repeat(80));
  console.log(`📊 SUSTAINED LOAD TEST - Elapsed: ${elapsed}s - Phase: ${state.phase.toUpperCase()}`);
  console.log('='.repeat(80));
  console.log('\n🔌 Connections:');
  console.log(`   Active:       ${state.activeConnections.size} / ${CONFIG.TARGET_CONNECTIONS} target`);
  console.log(`   Created:      ${state.totalConnectionsCreated}`);
  console.log(`   Failed:       ${state.failedConnections}`);
  console.log(`   Success Rate: ${successRate}%`);
  console.log(`   Server Reports: ${serverConns} active`);

  console.log('\n📨 Messages:');
  console.log(`   Received:     ${state.messagesReceived.toLocaleString()}`);
  console.log(`   Rate:         ${msgRate.toFixed(2)} msg/sec`);
  console.log(`   Errors:       ${state.errors} (${(state.errors / (state.messagesReceived || 1) * 100).toFixed(2)}%)`);

  console.log('\n💻 Server Health:');
  console.log(`   Status:       ${health?.healthy ? '✅ Healthy' : '❌ Unhealthy'}`);
  console.log(`   CPU:          ${cpuUsage}%`);
  console.log(`   Memory:       ${memUsage}%`);

  if (state.phase === 'ramping') {
    const rampElapsed = Math.floor((Date.now() - state.rampStartTime) / 1000);
    const rampProgress = (state.totalConnectionsCreated / CONFIG.TARGET_CONNECTIONS * 100).toFixed(1);
    console.log('\n🚀 Ramp Progress:');
    console.log(`   Progress:     ${rampProgress}%`);
    console.log(`   Time:         ${rampElapsed}s`);
  } else if (state.phase === 'sustaining') {
    console.log('\n🔒 Sustain Status:');
    console.log(`   Elapsed:      ${elapsedSustain}s`);
    console.log(`   Remaining:    ${remainingSustain}s`);
  }

  if (Object.keys(state.connectionErrors).length > 0) {
    console.log('\n⚠️  Connection Errors:');
    for (const [code, count] of Object.entries(state.connectionErrors)) {
      console.log(`   ${code}: ${count}`);
    }
  }

  console.log('='.repeat(80) + '\n');
}

// ============================================================================
// MAIN TEST EXECUTION
// ============================================================================

async function runTest() {
  console.log('\n' + '='.repeat(80));
  console.log('🧪 SUSTAINED LOAD TEST');
  console.log('='.repeat(80));

  // Determine test mode based on configuration
  const serverMaxConnections = 500; // From WS_MAX_CONNECTIONS in docker-compose.yml
  let testMode = 'CAPACITY TEST';
  let testModeEmoji = '📊';
  let testModeDescription = 'Testing at server capacity limit';

  if (CONFIG.TARGET_CONNECTIONS > serverMaxConnections) {
    testMode = 'STRESS/OVERLOAD TEST';
    testModeEmoji = '⚠️';
    testModeDescription = `Intentional overload (${CONFIG.TARGET_CONNECTIONS} > ${serverMaxConnections} limit)`;
  } else if (CONFIG.RAMP_RATE >= 500) {
    testMode = 'BURST/SPIKE TEST';
    testModeEmoji = '⚡';
    testModeDescription = `Rapid connection burst (${CONFIG.RAMP_RATE} conn/sec)`;
  }

  console.log(`\n${testModeEmoji} TEST MODE: ${testMode}`);
  console.log(`   ${testModeDescription}`);
  console.log(`\n📋 Configuration:`);
  console.log(`   Target:       ${CONFIG.TARGET_CONNECTIONS} connections`);
  console.log(`   Server Limit: ${serverMaxConnections} connections (WS_MAX_CONNECTIONS)`);
  console.log(`   Ramp Rate:    ${CONFIG.RAMP_RATE} conn/sec`);
  console.log(`   Sustain:      ${CONFIG.SUSTAIN_DURATION_MS / 1000}s (${Math.floor(CONFIG.SUSTAIN_DURATION_MS / 60000)} minutes)`);
  console.log(`   Server:       ${CONFIG.WS_URL}`);
  console.log(`   Health:       ${CONFIG.HEALTH_URL}`);
  console.log('\n' + '='.repeat(80) + '\n');

  // Initial health check
  log('🏥 Performing initial health check...', 'INFO');
  const initialHealth = await checkServerHealth();
  if (!initialHealth) {
    log('❌ Server health check failed! Aborting test.', 'ERROR');
    process.exit(1);
  }

  if (!initialHealth.healthy) {
    log('⚠️  Server reports unhealthy status but continuing...', 'WARN');
  }

  // Start periodic health checks
  const healthInterval = setInterval(async () => {
    await checkServerHealth();
  }, CONFIG.HEALTH_CHECK_INTERVAL_MS);

  // Start periodic reporting
  const reportInterval = setInterval(() => {
    printReport();
  }, CONFIG.REPORT_INTERVAL_MS);

  // Start ramp-up
  await rampUpConnections();

  // Wait for sustain duration
  if (state.phase === 'sustaining') {
    await new Promise(resolve => {
      setTimeout(() => {
        state.phase = 'completed';
        resolve();
      }, CONFIG.SUSTAIN_DURATION_MS);
    });
  }

  // Cleanup
  clearInterval(healthInterval);
  clearInterval(reportInterval);

  // Final report
  log('✅ Test completed!', 'INFO');
  printReport();

  // Close connections
  closeAllConnections();

  // Wait for connections to close
  await new Promise(resolve => setTimeout(resolve, 2000));

  log('🎉 Sustained load test finished!', 'INFO');
  process.exit(0);
}

// Graceful shutdown
process.on('SIGINT', () => {
  log('🛑 Received SIGINT, gracefully shutting down...', 'INFO');
  printReport();
  closeAllConnections();
  process.exit(0);
});

process.on('SIGTERM', () => {
  log('🛑 Received SIGTERM, gracefully shutting down...', 'INFO');
  printReport();
  closeAllConnections();
  process.exit(0);
});

// Run test
runTest().catch(error => {
  log(`❌ Test failed: ${error.message}`, 'ERROR');
  console.error(error);
  process.exit(1);
});
