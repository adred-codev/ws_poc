#!/usr/bin/env node

/**
 * Realistic Crypto Trading Traffic Simulator
 *
 * Simulates real-world trading platform usage:
 * - Gradual connection ramp-up
 * - Variable session lengths (short traders, long holders, 24/7 bots)
 * - Peak hours simulation
 * - Auto-balances based on server resources
 * - Monitors health and adjusts load
 */

const WebSocket = require('ws');
const http = require('http');

// ============================================================================
// CONFIGURATION
// ============================================================================

const CONFIG = {
  // Server settings
  WS_URL: process.env.WS_URL || 'ws://localhost:3004/ws',
  HEALTH_URL: process.env.HEALTH_URL || 'http://localhost:3004/health',

  // Target connections (will auto-adjust based on server health)
  TARGET_CONNECTIONS: parseInt(process.env.TARGET_CONNECTIONS) || 1000,
  MIN_CONNECTIONS: parseInt(process.env.MIN_CONNECTIONS) || 100,
  MAX_CONNECTIONS: parseInt(process.env.MAX_CONNECTIONS) || 5000,

  // Ramp-up settings (gradual increase, not instant)
  RAMP_UP_DURATION_MS: 60000, // 1 minute to reach target
  CONNECTIONS_PER_WAVE: 10,   // Add 10 connections at a time
  WAVE_INTERVAL_MS: 500,       // Every 500ms

  // Session duration profiles (realistic trading patterns)
  SESSION_PROFILES: {
    QUICK_TRADER: { min: 30000, max: 300000 },      // 30s - 5min
    ACTIVE_TRADER: { min: 300000, max: 1800000 },   // 5min - 30min
    DAY_TRADER: { min: 1800000, max: 14400000 },    // 30min - 4hours
    LONG_HOLDER: { min: 14400000, max: 28800000 },  // 4hours - 8hours
    BOT_247: { min: 86400000, max: 172800000 }      // 24hours - 48hours
  },

  // Distribution of trader types (percentages)
  TRADER_DISTRIBUTION: {
    QUICK_TRADER: 0.20,  // 20% quick in/out
    ACTIVE_TRADER: 0.35, // 35% active traders
    DAY_TRADER: 0.30,    // 30% day traders
    LONG_HOLDER: 0.10,   // 10% long holders
    BOT_247: 0.05        // 5% bots/algorithms
  },

  // Health check settings
  HEALTH_CHECK_INTERVAL_MS: 5000,  // Check every 5 seconds

  // Auto-scaling thresholds
  SCALE_DOWN_CPU_THRESHOLD: 85,    // Scale down if CPU > 85%
  SCALE_DOWN_MEMORY_THRESHOLD: 90, // Scale down if memory > 90%
  SCALE_DOWN_ERROR_RATE: 0.05,     // Scale down if error rate > 5%
  SCALE_UP_CPU_THRESHOLD: 50,      // Scale up if CPU < 50%
  SCALE_UP_MEMORY_THRESHOLD: 60,   // Scale up if memory < 60%

  // Reconnect settings
  RECONNECT_PROBABILITY: 0.7,      // 70% of disconnected users reconnect
  RECONNECT_DELAY_MS: 5000,        // Wait 5s before reconnecting

  // Peak hours simulation (UTC hours)
  PEAK_HOURS: [13, 14, 15, 16, 17, 18, 19, 20], // Market peak hours
  PEAK_MULTIPLIER: 1.5,  // 50% more connections during peak

  // Test duration
  DURATION_MS: parseInt(process.env.DURATION) * 1000 || 3600000, // Default 1 hour
};

// ============================================================================
// STATE MANAGEMENT
// ============================================================================

const state = {
  // Connection tracking
  activeConnections: new Map(),
  totalConnectionsCreated: 0,
  totalConnectionsClosed: 0,
  totalReconnections: 0,

  // Metrics
  messagesReceived: 0,
  errors: 0,
  connectionErrors: 0,

  // Health monitoring
  lastHealthCheck: null,
  serverHealthy: true,
  currentTargetConnections: CONFIG.TARGET_CONNECTIONS,

  // Timing
  startTime: Date.now(),
  lastReportTime: Date.now(),

  // Control
  isRampingUp: true,
  isShuttingDown: false,
};

// ============================================================================
// UTILITY FUNCTIONS
// ============================================================================

function log(message, level = 'INFO') {
  const timestamp = new Date().toISOString();
  const elapsed = ((Date.now() - state.startTime) / 1000).toFixed(1);
  console.log(`[${timestamp}] [${level}] [+${elapsed}s] ${message}`);
}

function randomBetween(min, max) {
  return Math.floor(Math.random() * (max - min + 1)) + min;
}

function selectTraderType() {
  const rand = Math.random();
  let cumulative = 0;

  for (const [type, probability] of Object.entries(CONFIG.TRADER_DISTRIBUTION)) {
    cumulative += probability;
    if (rand <= cumulative) {
      return type;
    }
  }

  return 'ACTIVE_TRADER'; // Fallback
}

function getSessionDuration(traderType) {
  const profile = CONFIG.SESSION_PROFILES[traderType];
  return randomBetween(profile.min, profile.max);
}

function isPeakHour() {
  const currentHour = new Date().getUTCHours();
  return CONFIG.PEAK_HOURS.includes(currentHour);
}

function calculateTargetConnections() {
  let target = CONFIG.TARGET_CONNECTIONS;

  // Adjust for peak hours
  if (isPeakHour()) {
    target = Math.floor(target * CONFIG.PEAK_MULTIPLIER);
  }

  // Clamp to min/max
  return Math.max(
    CONFIG.MIN_CONNECTIONS,
    Math.min(CONFIG.MAX_CONNECTIONS, target)
  );
}

// ============================================================================
// HEALTH MONITORING
// ============================================================================

async function checkServerHealth() {
  try {
    const response = await fetch(CONFIG.HEALTH_URL);
    const data = await response.json();

    state.lastHealthCheck = data;
    state.serverHealthy = data.healthy;

    // Extract metrics
    const cpuPercentage = data.checks?.cpu?.percentage || 0;
    const memoryPercentage = data.checks?.memory?.percentage || 0;
    const connectionPercentage = data.checks?.capacity?.percentage || 0;

    // Calculate error rate
    const errorRate = state.errors / (state.messagesReceived + state.errors || 1);

    // Auto-scaling logic
    const shouldScaleDown =
      cpuPercentage > CONFIG.SCALE_DOWN_CPU_THRESHOLD ||
      memoryPercentage > CONFIG.SCALE_DOWN_MEMORY_THRESHOLD ||
      errorRate > CONFIG.SCALE_DOWN_ERROR_RATE ||
      !data.healthy;

    const shouldScaleUp =
      data.healthy &&
      cpuPercentage < CONFIG.SCALE_UP_CPU_THRESHOLD &&
      memoryPercentage < CONFIG.SCALE_UP_MEMORY_THRESHOLD &&
      state.activeConnections.size < state.currentTargetConnections;

    if (shouldScaleDown && state.activeConnections.size > CONFIG.MIN_CONNECTIONS) {
      // Reduce target by 10%
      const reduction = Math.floor(state.activeConnections.size * 0.1);
      state.currentTargetConnections = Math.max(
        CONFIG.MIN_CONNECTIONS,
        state.activeConnections.size - reduction
      );
      log(`🔽 Scaling DOWN: Server under stress (CPU: ${cpuPercentage.toFixed(1)}%, Mem: ${memoryPercentage.toFixed(1)}%, Errors: ${(errorRate * 100).toFixed(2)}%)`, 'WARN');
      log(`   Target connections: ${state.currentTargetConnections}`, 'WARN');

      // Disconnect some connections
      disconnectRandom(reduction);
    } else if (shouldScaleUp && !state.isRampingUp) {
      // Increase target by 5%
      const increase = Math.floor(state.activeConnections.size * 0.05);
      state.currentTargetConnections = Math.min(
        calculateTargetConnections(),
        state.activeConnections.size + increase
      );
      log(`🔼 Scaling UP: Server healthy, adding connections (CPU: ${cpuPercentage.toFixed(1)}%, Mem: ${memoryPercentage.toFixed(1)}%)`);
      log(`   Target connections: ${state.currentTargetConnections}`);
    }

    return data;
  } catch (error) {
    log(`❌ Health check failed: ${error.message}`, 'ERROR');
    state.serverHealthy = false;
    return null;
  }
}

function disconnectRandom(count) {
  const connections = Array.from(state.activeConnections.values());
  const toDisconnect = connections
    .sort(() => Math.random() - 0.5)
    .slice(0, count);

  toDisconnect.forEach(conn => {
    try {
      conn.ws.close(1000, 'Auto-scaling down');
    } catch (error) {
      // Ignore errors
    }
  });
}

// ============================================================================
// CONNECTION MANAGEMENT
// ============================================================================

class TradingConnection {
  constructor(id) {
    this.id = id;
    this.traderType = selectTraderType();
    this.sessionDuration = getSessionDuration(this.traderType);
    this.connectTime = Date.now();
    this.messagesReceived = 0;
    this.ws = null;
    this.disconnectTimer = null;
    this.reconnectTimer = null;
    this.willReconnect = Math.random() < CONFIG.RECONNECT_PROBABILITY;
  }

  connect() {
    try {
      this.ws = new WebSocket(CONFIG.WS_URL);

      this.ws.on('open', () => this.onOpen());
      this.ws.on('message', (data) => this.onMessage(data));
      this.ws.on('error', (error) => this.onError(error));
      this.ws.on('close', (code, reason) => this.onClose(code, reason));

    } catch (error) {
      this.onError(error);
    }
  }

  onOpen() {
    state.activeConnections.set(this.id, this);
    state.totalConnectionsCreated++;

    // Schedule automatic disconnect based on session duration
    this.disconnectTimer = setTimeout(() => {
      this.disconnect('Session timeout');
    }, this.sessionDuration);
  }

  onMessage(data) {
    this.messagesReceived++;
    state.messagesReceived++;

    // Parse and validate message (basic check)
    try {
      const message = JSON.parse(data.toString());
      if (!message.type || !message.token) {
        state.errors++;
      }
    } catch (error) {
      state.errors++;
    }
  }

  onError(error) {
    state.connectionErrors++;
    state.errors++;

    if (error.code === 'ECONNREFUSED') {
      log(`❌ Connection ${this.id} failed: Server not available`, 'ERROR');
      state.serverHealthy = false;
    }
  }

  onClose(code, reason) {
    state.totalConnectionsClosed++;
    state.activeConnections.delete(this.id);

    if (this.disconnectTimer) {
      clearTimeout(this.disconnectTimer);
    }

    // Reconnect logic (simulates users coming back)
    if (this.willReconnect && !state.isShuttingDown) {
      this.reconnectTimer = setTimeout(() => {
        state.totalReconnections++;
        const newConn = new TradingConnection(this.id + '_R' + state.totalReconnections);
        newConn.connect();
      }, CONFIG.RECONNECT_DELAY_MS);
    }
  }

  disconnect(reason = 'Manual disconnect') {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.close(1000, reason);
    }
  }
}

// ============================================================================
// RAMP-UP MANAGER
// ============================================================================

async function rampUpConnections() {
  const targetConnections = calculateTargetConnections();
  state.currentTargetConnections = targetConnections;

  log(`🚀 Starting ramp-up to ${targetConnections} connections`);
  log(`📊 Trader distribution: Quick(${(CONFIG.TRADER_DISTRIBUTION.QUICK_TRADER*100).toFixed(0)}%) Active(${(CONFIG.TRADER_DISTRIBUTION.ACTIVE_TRADER*100).toFixed(0)}%) Day(${(CONFIG.TRADER_DISTRIBUTION.DAY_TRADER*100).toFixed(0)}%) Long(${(CONFIG.TRADER_DISTRIBUTION.LONG_HOLDER*100).toFixed(0)}%) Bots(${(CONFIG.TRADER_DISTRIBUTION.BOT_247*100).toFixed(0)}%)`);
  log(`⏰ Peak hours (UTC): ${CONFIG.PEAK_HOURS.join(', ')}`);

  const rampUpInterval = setInterval(() => {
    if (state.isShuttingDown) {
      clearInterval(rampUpInterval);
      return;
    }

    const currentCount = state.activeConnections.size;
    const target = state.currentTargetConnections;

    if (currentCount >= target) {
      clearInterval(rampUpInterval);
      state.isRampingUp = false;
      log(`✅ Ramp-up complete: ${currentCount} connections active`);
      return;
    }

    // Don't add more if server is unhealthy
    if (!state.serverHealthy) {
      log(`⚠️  Pausing ramp-up: Server unhealthy`, 'WARN');
      return;
    }

    // Add wave of connections
    const toAdd = Math.min(
      CONFIG.CONNECTIONS_PER_WAVE,
      target - currentCount
    );

    for (let i = 0; i < toAdd; i++) {
      const connId = `conn_${state.totalConnectionsCreated + i + 1}`;
      const conn = new TradingConnection(connId);
      conn.connect();
    }

  }, CONFIG.WAVE_INTERVAL_MS);
}

// ============================================================================
// REPORTING
// ============================================================================

function printReport() {
  const now = Date.now();
  const elapsed = (now - state.startTime) / 1000;
  const duration = (now - state.lastReportTime) / 1000;

  const activeCount = state.activeConnections.size;
  const messagesRate = (state.messagesReceived / elapsed).toFixed(2);
  const errorRate = ((state.errors / (state.messagesReceived + state.errors || 1)) * 100).toFixed(2);

  const health = state.lastHealthCheck;
  const cpuUsage = health?.checks?.cpu?.percentage?.toFixed(1) || 'N/A';
  const memoryUsage = health?.checks?.memory?.percentage?.toFixed(1) || 'N/A';
  const serverConnections = health?.checks?.capacity?.current || 'N/A';

  console.log('\n' + '='.repeat(80));
  console.log(`📊 SIMULATION REPORT - Elapsed: ${elapsed.toFixed(0)}s`);
  console.log('='.repeat(80));

  console.log('\n🔌 Connections:');
  console.log(`   Active:       ${activeCount} / ${state.currentTargetConnections} target`);
  console.log(`   Total Created: ${state.totalConnectionsCreated}`);
  console.log(`   Total Closed:  ${state.totalConnectionsClosed}`);
  console.log(`   Reconnections: ${state.totalReconnections}`);
  console.log(`   Server Reports: ${serverConnections} active`);

  console.log('\n📨 Messages:');
  console.log(`   Received:     ${state.messagesReceived.toLocaleString()}`);
  console.log(`   Rate:         ${messagesRate} msg/sec`);
  console.log(`   Errors:       ${state.errors} (${errorRate}%)`);

  console.log('\n💻 Server Health:');
  console.log(`   Status:       ${state.serverHealthy ? '✅ Healthy' : '❌ Unhealthy'}`);
  console.log(`   CPU:          ${cpuUsage}%`);
  console.log(`   Memory:       ${memoryUsage}%`);

  console.log('\n👥 Trader Types Distribution:');
  const traderCounts = {};
  for (const conn of state.activeConnections.values()) {
    traderCounts[conn.traderType] = (traderCounts[conn.traderType] || 0) + 1;
  }
  for (const [type, count] of Object.entries(traderCounts)) {
    const percentage = ((count / activeCount) * 100).toFixed(1);
    console.log(`   ${type.padEnd(15)}: ${count.toString().padStart(4)} (${percentage}%)`);
  }

  console.log('\n⏰ Time Info:');
  console.log(`   Current Hour:  ${new Date().getUTCHours()} UTC ${isPeakHour() ? '🔥 PEAK' : ''}`);
  console.log(`   Remaining:     ${((CONFIG.DURATION_MS - (now - state.startTime)) / 1000).toFixed(0)}s`);

  console.log('='.repeat(80) + '\n');

  state.lastReportTime = now;
}

// ============================================================================
// SHUTDOWN
// ============================================================================

function shutdown() {
  if (state.isShuttingDown) return;

  state.isShuttingDown = true;
  log('🛑 Shutting down simulator...');

  // Disconnect all active connections
  for (const conn of state.activeConnections.values()) {
    conn.disconnect('Simulator shutdown');
  }

  // Final report
  setTimeout(() => {
    printReport();
    log('👋 Shutdown complete');
    process.exit(0);
  }, 2000);
}

// ============================================================================
// MAIN
// ============================================================================

async function main() {
  log('🎬 Starting Realistic Trading Traffic Simulator');
  log(`📍 WebSocket URL: ${CONFIG.WS_URL}`);
  log(`🎯 Target Connections: ${CONFIG.TARGET_CONNECTIONS}`);
  log(`⏱️  Test Duration: ${CONFIG.DURATION_MS / 1000}s`);

  // Check server is available
  log('🔍 Checking server health...');
  const initialHealth = await checkServerHealth();

  if (!initialHealth) {
    log('❌ Server is not reachable. Exiting.', 'ERROR');
    process.exit(1);
  }

  log('✅ Server is healthy, starting simulation');

  // Start health monitoring
  const healthCheckInterval = setInterval(async () => {
    await checkServerHealth();
  }, CONFIG.HEALTH_CHECK_INTERVAL_MS);

  // Start reporting
  const reportInterval = setInterval(() => {
    printReport();
  }, 10000); // Report every 10 seconds

  // Start ramp-up
  await rampUpConnections();

  // Simulate peak hours check (every minute)
  const peakHourCheck = setInterval(() => {
    const newTarget = calculateTargetConnections();
    if (newTarget !== state.currentTargetConnections) {
      const wasPeak = state.currentTargetConnections > CONFIG.TARGET_CONNECTIONS;
      state.currentTargetConnections = newTarget;

      if (wasPeak) {
        log(`🌙 Exiting peak hours, reducing target to ${newTarget}`);
      } else {
        log(`🔥 Entering peak hours, increasing target to ${newTarget}`);
      }
    }
  }, 60000); // Check every minute

  // Duration timeout
  setTimeout(() => {
    log('⏰ Test duration complete');
    clearInterval(healthCheckInterval);
    clearInterval(reportInterval);
    clearInterval(peakHourCheck);
    shutdown();
  }, CONFIG.DURATION_MS);

  // Handle signals
  process.on('SIGINT', () => {
    log('\n🛑 Received SIGINT');
    clearInterval(healthCheckInterval);
    clearInterval(reportInterval);
    clearInterval(peakHourCheck);
    shutdown();
  });

  process.on('SIGTERM', () => {
    log('\n🛑 Received SIGTERM');
    clearInterval(healthCheckInterval);
    clearInterval(reportInterval);
    clearInterval(peakHourCheck);
    shutdown();
  });
}

// Start simulator
main().catch(error => {
  log(`💥 Fatal error: ${error.message}`, 'ERROR');
  console.error(error);
  process.exit(1);
});
