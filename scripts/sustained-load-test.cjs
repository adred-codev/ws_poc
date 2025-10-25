#!/usr/bin/env node

/**
 * Sustained Load Test for WebSocket Server with Coarse-Grained Channel Subscriptions
 *
 * Purpose: Test server stability under sustained load with realistic subscription patterns
 * - Gradual ramp-up (avoid burst rejections)
 * - Hold connections stable (no churn)
 * - Active message flow throughout
 * - Monitor metrics over time
 * - Support coarse-grained channel subscriptions (token.BTC, user.alice, global)
 * - Multiple subscription distribution modes (all, single, random)
 *
 * COARSE-GRAINED SUBSCRIPTION FILTERING
 * --------------------------------------
 * Each client can subscribe to specific channels instead of receiving all messages.
 * Server uses sharded subscription index for efficient routing (16x lock contention reduction).
 *
 * Channel Format: {type}.{identifier} or global
 * - TYPES: token, user
 * - Examples: token.BTC (all events for BTC), user.alice (all events for user), global (system-wide)
 * - Event types in message payload: {type: 'token:trade', tokenId: 'BTC', ...}
 *
 * Environment Variables:
 * - CHANNELS: Comma-separated list of channels (default: token.BTC,token.ETH,token.SOL,token.ODIN,token.DOGE)
 * - SUBSCRIPTION_MODE: Distribution strategy - 'all', 'single', or 'random' (default: all)
 * - CHANNELS_PER_CLIENT: For 'random' mode, how many channels per client (default: 3)
 *
 * Examples:
 * - All clients subscribe to all token channels (default):
 *   npm run test:sustained
 *
 * - Each client subscribes to ONE channel (round-robin):
 *   SUBSCRIPTION_MODE=single TARGET_CONNECTIONS=5000 npm run test:sustained
 *
 * - Each client subscribes to 2 random channels:
 *   SUBSCRIPTION_MODE=random CHANNELS=token.BTC,token.ETH,token.SOL CHANNELS_PER_CLIENT=2 TARGET_CONNECTIONS=5000 npm run test:sustained
 *
 * - Test mixed channel types:
 *   CHANNELS=token.BTC,token.ETH,user.alice,global SUBSCRIPTION_MODE=random CHANNELS_PER_CLIENT=2 npm run test:sustained
 *
 * - Test without subscriptions (receive all messages):
 *   CHANNELS= npm run test:sustained
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
//   - WS_MAX_CONNECTIONS: 18000 (hard connection limit)
//   - WS_MAX_NATS_RATE: 20 messages/sec (rate limit)
//   - WS_CPU_REJECT_THRESHOLD: 75% (emergency brake)
//
// TEST MODES:
//
// 1. CAPACITY TEST (recommended default):
//    - Set TARGET_CONNECTIONS = server's WS_MAX_CONNECTIONS (18000)
//    - Set RAMP_RATE to gradual value (100-200 conn/sec)
//    - Expected: All connections succeed, server stable, no rejections
//    - Purpose: Verify server handles maximum configured load
//    - Example: TARGET_CONNECTIONS=18000 RAMP_RATE=100 npm run test:sustained
//
// 2. STRESS/OVERLOAD TEST (intentional overload):
//    - Set TARGET_CONNECTIONS > server's WS_MAX_CONNECTIONS (e.g., 3000-4000)
//    - Set RAMP_RATE higher (200-300 conn/sec)
//    - Expected: Rejections once limit hit, server stays stable, no crash
//    - Purpose: Verify server correctly rejects excess load without crashing
//    - Example: TARGET_CONNECTIONS=3000 RAMP_RATE=200 npm run test:sustained
//
// 3. BURST/SPIKE TEST (rapid connection attempts):
//    - Set TARGET_CONNECTIONS to server limit (18000)
//    - Set RAMP_RATE very high (1000-2000 conn/sec)
//    - Expected: Some rejections during burst, server recovers, final count = limit
//    - Purpose: Verify server handles traffic spikes gracefully
//    - Example: TARGET_CONNECTIONS=18000 RAMP_RATE=1000 npm run test:sustained
//
// CONNECTION TIMEOUT:
//    - Default: 10 seconds (industry standard for production systems)
//    - Override with CONNECTION_TIMEOUT env var (in milliseconds)
//    - Example: CONNECTION_TIMEOUT=15000 npm run test:sustained (15s timeout)
//    - Note: 5s is too aggressive for realistic load testing (users wait 10-30s)
//
// ============================================================================

const CONFIG = {
  // Server settings
  WS_URL: process.env.WS_URL || 'ws://localhost:3004/ws',
  HEALTH_URL: process.env.HEALTH_URL || 'http://localhost:3004/health',

  // Target connections
  // DEFAULT: Matches server's WS_MAX_CONNECTIONS for capacity testing
  // Override with TARGET_CONNECTIONS env var for stress testing
  TARGET_CONNECTIONS: parseInt(process.env.TARGET_CONNECTIONS) || 18000,

  // Ramp-up settings (gradual increase)
  // DEFAULT: Moderate ramp for smooth capacity testing
  // Override with RAMP_RATE env var for burst/stress testing
  RAMP_RATE: parseInt(process.env.RAMP_RATE) || 100, // connections per second

  // Sustain duration (after reaching target)
  // DEFAULT: 30 minutes to verify long-term stability
  // Override with DURATION env var (in seconds) for shorter tests
  SUSTAIN_DURATION_MS: parseInt(process.env.DURATION) * 1000 || 1800000, // Default 30 min

  // Reporting interval
  REPORT_INTERVAL_MS: 10000, // Report every 10 seconds

  // Health check interval
  HEALTH_CHECK_INTERVAL_MS: 5000, // Check every 5 seconds

  // Subscription settings
  // Coarse-grained channels to subscribe to (comma-separated in env var)
  // Format: token.{SYMBOL}, user.{USER_ID}, or global
  // DEFAULT: token.BTC,token.ETH,token.SOL,token.ODIN,token.DOGE (all events for these tokens)
  // Example: CHANNELS=token.BTC,token.ETH,user.alice npm run test:sustained
  // To test WITHOUT subscriptions (receive all): CHANNELS= npm run test:sustained
  CHANNELS: (process.env.CHANNELS || 'token.BTC,token.ETH,token.SOL,token.ODIN,token.DOGE').split(',').filter(c => c.trim()),

  // Subscription distribution strategy:
  // - 'all': All clients subscribe to ALL channels (default)
  // - 'random': Each client subscribes to random subset of channels
  // - 'single': Each client subscribes to only ONE channel (round-robin)
  // Example: SUBSCRIPTION_MODE=random npm run test:sustained
  SUBSCRIPTION_MODE: process.env.SUBSCRIPTION_MODE || 'all',

  // For 'random' mode: how many channels per client (1-N)
  // Example: SUBSCRIPTION_MODE=random CHANNELS_PER_CLIENT=2 npm run test:sustained
  CHANNELS_PER_CLIENT: parseInt(process.env.CHANNELS_PER_CLIENT) || 3,

  // Connection timeout (default: 10s - industry standard)
  // Why 10 seconds?
  // - Real users wait 10-30s before giving up (not 5s "impatient developer timeout")
  // - Industry standard: AWS ELB (60s), Cloudflare (100s), Socket.io (20s), SignalR (15s)
  // - Load testing: Must match production client behavior for accurate capacity testing
  // - Goroutine scheduling: Server needs time to schedule new goroutines during load spikes
  //   (spawning 1000 connections = 2000 goroutines while managing 10K+ existing = 2-8s normal)
  // - 5s timeout was testing "how many connect in 5s?" instead of "what's true capacity?"
  // Override with CONNECTION_TIMEOUT env var (in milliseconds)
  CONNECTION_TIMEOUT_MS: parseInt(process.env.CONNECTION_TIMEOUT) || 10000,
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

  // Subscription tracking
  subscriptionsSent: 0,
  subscriptionsConfirmed: 0,
  subscriptionsFailed: 0,
  messagesFilteredOut: 0, // Messages received before subscription (shouldn't happen with filtering)

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

    // PERFORMANCE: Pre-allocate buffer for message processing
    // Reduces GC pressure by reusing string buffers
    this.messageBuffer = [];
    this.processingBatch = false;

    // Subscription management
    this.subscribedChannels = [];
    this.subscribed = false;
    this.subscriptionPending = false;
  }

  connect() {
    return new Promise((resolve) => {
      try {
        // PERFORMANCE OPTIMIZATION: Configure WebSocket for high throughput
        this.ws = new WebSocket(CONFIG.WS_URL, {
          // Disable compression for speed (we're testing throughput, not bandwidth)
          perMessageDeflate: false,

          // Increase buffer sizes for better throughput
          // Default is 16KB, we increase to 512KB to handle bursts
          maxPayload: 512 * 1024, // 512KB max message size

          // Disable auto-ping (we handle heartbeats manually)
          skipUTF8Validation: true, // Skip UTF-8 validation for speed
        });

        this.ws.on('open', () => {
          this.connected = true;
          this.connectTime = Date.now();
          state.activeConnections.set(this.id, this);
          this.startHeartbeat(); // Start sending heartbeats to prevent timeout

          // Auto-subscribe to channels if configured
          if (CONFIG.CHANNELS.length > 0) {
            this.autoSubscribe();
          }

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

        // Connection timeout (configurable, default 10s)
        setTimeout(() => {
          if (!this.connected) {
            this.ws.terminate();
            resolve(false);
          }
        }, CONFIG.CONNECTION_TIMEOUT_MS);

      } catch (error) {
        this.onError(error);
        resolve(false);
      }
    });
  }

  onMessage(data) {
    try {
      const message = JSON.parse(data.toString());

      // Handle subscription acknowledgments separately (don't count as regular messages)
      if (message.type === 'subscription_ack') {
        this.subscribed = true;
        this.subscriptionPending = false;
        state.subscriptionsConfirmed++;
        return;
      }

      // Handle unsubscription acknowledgments
      if (message.type === 'unsubscription_ack') {
        return;
      }

      // Handle pong messages
      if (message.type === 'pong') {
        return;
      }

      // Count regular messages
      this.messagesReceived++;
      state.messagesReceived++;

      // If we received a message before subscribing, count it as filtered out
      // (This shouldn't happen with proper server-side filtering)
      if (CONFIG.CHANNELS.length > 0 && !this.subscribed) {
        state.messagesFilteredOut++;
      }

    } catch (error) {
      // If parsing fails, treat as regular message for batched processing
      this.messagesReceived++;
      state.messagesReceived++;
    }

    // PERFORMANCE OPTIMIZATION: Batch message processing
    // Instead of processing each message individually (expensive),
    // collect messages in buffer and process in batches
    // This reduces:
    // 1. Number of setImmediate() calls (context switches)
    // 2. JSON parsing overhead per message
    // 3. Event loop pressure

    this.messageBuffer.push(data);

    // Process batch when we have 10 messages or if not already processing
    if (this.messageBuffer.length >= 10 || !this.processingBatch) {
      this.processBatchedMessages();
    }
  }

  processBatchedMessages() {
    if (this.processingBatch || this.messageBuffer.length === 0) {
      return;
    }

    this.processingBatch = true;

    // Defer batch processing to next tick
    setImmediate(() => {
      const batch = this.messageBuffer.splice(0, this.messageBuffer.length);

      for (const data of batch) {
        try {
          const message = JSON.parse(data.toString());
          if (!message.type) {
            state.errors++;
          }
        } catch (error) {
          state.errors++;
        }
      }

      this.processingBatch = false;

      // If more messages arrived while processing, schedule next batch
      if (this.messageBuffer.length > 0) {
        this.processBatchedMessages();
      }
    });
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

    // PERFORMANCE: Clean up message buffer to prevent memory leak
    this.messageBuffer = [];
    this.processingBatch = false;
  }

  startHeartbeat() {
    // Send heartbeat every 30 seconds to prevent server read timeout (60s)
    this.heartbeatInterval = setInterval(() => {
      if (this.ws && this.ws.readyState === WebSocket.OPEN) {
        try {
          this.ws.send(JSON.stringify({ type: 'heartbeat' }));
        } catch (error) {
          // ROOT CAUSE FIX: Connection is dead - detect and close it properly
          // Previously ignored send errors, causing dead connections to remain tracked
          // as "active" and inflating connection counts, explaining the "dwindling" effect
          //
          // When TCP connection becomes stale (e.g., GCP network infrastructure drops it),
          // the WebSocket send() will fail. We must close the connection properly so the
          // onClose() handler can clean up state.activeConnections tracking.
          console.log(`⚠️  Connection ${this.id} dead (heartbeat send failed): ${error.message}`);
          this.close();
        }
      }
    }, 30000); // 30 seconds (half of server's 60s timeout)
  }

  /**
   * Auto-subscribe based on configured distribution mode
   */
  autoSubscribe() {
    let channelsToSubscribe = [];

    switch (CONFIG.SUBSCRIPTION_MODE) {
      case 'all':
        // All clients subscribe to all channels
        channelsToSubscribe = CONFIG.CHANNELS;
        break;

      case 'single':
        // Each client subscribes to one channel (round-robin)
        const channelIndex = this.id % CONFIG.CHANNELS.length;
        channelsToSubscribe = [CONFIG.CHANNELS[channelIndex]];
        break;

      case 'random':
        // Each client subscribes to random subset of channels
        const numChannels = Math.min(CONFIG.CHANNELS_PER_CLIENT, CONFIG.CHANNELS.length);
        const shuffled = [...CONFIG.CHANNELS].sort(() => Math.random() - 0.5);
        channelsToSubscribe = shuffled.slice(0, numChannels);
        break;

      default:
        channelsToSubscribe = CONFIG.CHANNELS;
    }

    this.subscribe(channelsToSubscribe);
  }

  /**
   * Subscribe to specific channels
   */
  subscribe(channels) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN || this.subscriptionPending) {
      return;
    }

    try {
      const message = JSON.stringify({
        type: 'subscribe',
        data: { channels }
      });

      this.ws.send(message);
      this.subscribedChannels = channels;
      this.subscriptionPending = true;
      state.subscriptionsSent++;
    } catch (error) {
      state.subscriptionsFailed++;
    }
  }

  /**
   * Unsubscribe from specific channels
   */
  unsubscribe(channels) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      return;
    }

    try {
      const message = JSON.stringify({
        type: 'unsubscribe',
        data: { channels }
      });

      this.ws.send(message);
      this.subscribedChannels = this.subscribedChannels.filter(c => !channels.includes(c));
    } catch (error) {
      // Ignore errors
    }
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
  if (state.messagesFilteredOut > 0) {
    console.log(`   ⚠️  Pre-sub msgs: ${state.messagesFilteredOut} (should be 0 with filtering)`);
  }

  if (CONFIG.CHANNELS.length > 0) {
    console.log('\n🔔 Subscriptions:');
    console.log(`   Mode:         ${CONFIG.SUBSCRIPTION_MODE}`);
    console.log(`   Channels:     ${CONFIG.CHANNELS.join(', ')}`);
    console.log(`   Sent:         ${state.subscriptionsSent}`);
    console.log(`   Confirmed:    ${state.subscriptionsConfirmed}`);
    console.log(`   Failed:       ${state.subscriptionsFailed}`);
    const subRate = (state.subscriptionsConfirmed / state.subscriptionsSent * 100).toFixed(1);
    console.log(`   Success Rate: ${subRate}%`);
  }

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
  const serverMaxConnections = parseInt(process.env.WS_MAX_CONNECTIONS) || 18000;
  let testMode = 'CAPACITY TEST';
  let testModeEmoji = '📊';
  let testModeDescription = 'Testing at server capacity limit';

  if (CONFIG.TARGET_CONNECTIONS > serverMaxConnections) {
    testMode = 'STRESS/OVERLOAD TEST';
    testModeEmoji = '⚠️';
    testModeDescription = `Intentional overload (${CONFIG.TARGET_CONNECTIONS} > ${serverMaxConnections} limit)`;
  } else if (CONFIG.RAMP_RATE >= 1000) {
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
  console.log(`   Timeout:      ${CONFIG.CONNECTION_TIMEOUT_MS / 1000}s (connection timeout)`);
  console.log(`   Sustain:      ${CONFIG.SUSTAIN_DURATION_MS / 1000}s (${Math.floor(CONFIG.SUSTAIN_DURATION_MS / 60000)} minutes)`);
  console.log(`   Server:       ${CONFIG.WS_URL}`);
  console.log(`   Health:       ${CONFIG.HEALTH_URL}`);

  if (CONFIG.CHANNELS.length > 0) {
    console.log(`\n🔔 Subscription Settings:`);
    console.log(`   Mode:         ${CONFIG.SUBSCRIPTION_MODE}`);
    console.log(`   Channels:     ${CONFIG.CHANNELS.join(', ')} (${CONFIG.CHANNELS.length} total)`);
    if (CONFIG.SUBSCRIPTION_MODE === 'random') {
      console.log(`   Per Client:   ${CONFIG.CHANNELS_PER_CLIENT} channels`);
    }
    console.log(`   Impact:       Expected ${CONFIG.CHANNELS.length}x reduction in message fanout`);
  } else {
    console.log(`\n⚠️  Subscription Filtering: DISABLED (all clients receive all messages)`);
  }

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
