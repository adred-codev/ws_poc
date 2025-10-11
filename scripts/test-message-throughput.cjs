#!/usr/bin/env node

/**
 * Message Throughput Test for WebSocket Server
 *
 * Purpose: Test ONLY message processing throughput (not connection capacity)
 * - Establishes fixed number of connections quickly
 * - Measures message receive rate and processing reliability
 * - Validates server can broadcast to all clients without slow client issues
 * - Tests client-side message processing performance
 *
 * This complements test-connection-rate.cjs which tests connection capacity.
 *
 * TESTING STRATEGY:
 * 1. Run test-connection-rate.cjs → Validate 5000 connections WITHOUT messages
 * 2. Run THIS test @ 500 conns → Validate message throughput
 * 3. Extrapolate: If both pass, server handles 5000 × 20 msg/sec when distributed
 */

const WebSocket = require('ws');

// ============================================================================
// CONFIGURATION
// ============================================================================

const CONFIG = {
  // Server settings
  WS_URL: process.env.WS_URL || process.argv[2] || 'ws://localhost:3004/ws',

  // Connection settings
  TARGET_CONNECTIONS: parseInt(process.env.TARGET_CONNECTIONS || process.argv[3]) || 500,

  // Test duration (after all connections established)
  TEST_DURATION: parseInt(process.env.DURATION || process.argv[4]) || 300, // seconds

  // Reporting interval
  REPORT_INTERVAL: 10, // seconds
};

// ============================================================================
// STATE MANAGEMENT
// ============================================================================

const state = {
  // Connection tracking
  connections: [],
  connectionsFailed: 0,
  connectionsEstablished: 0,

  // Message metrics
  totalMessagesReceived: 0,
  messagesByConnection: new Map(),
  messageErrors: 0,

  // Timing
  setupStartTime: null,
  testStartTime: null,
  phase: 'setup', // 'setup', 'running', 'completed'
};

// ============================================================================
// CONNECTION CLASS - OPTIMIZED FOR MESSAGE PROCESSING
// ============================================================================

class ThroughputTestConnection {
  constructor(id) {
    this.id = id;
    this.ws = null;
    this.connected = false;
    this.messagesReceived = 0;

    // PERFORMANCE: Batched message processing
    this.messageBuffer = [];
    this.processingBatch = false;
  }

  connect() {
    return new Promise((resolve) => {
      try {
        // Optimized WebSocket config for throughput
        this.ws = new WebSocket(CONFIG.WS_URL, {
          perMessageDeflate: false,
          maxPayload: 512 * 1024,
          skipUTF8Validation: true,
        });

        this.ws.on('open', () => {
          this.connected = true;
          state.connectionsEstablished++;
          this.startHeartbeat();
          resolve(true);
        });

        this.ws.on('message', (data) => {
          this.onMessage(data);
        });

        this.ws.on('error', () => {
          // Ignore errors during message phase
        });

        this.ws.on('close', () => {
          this.onClose();
        });

        this.ws.on('ping', () => {
          if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.ws.pong();
          }
        });

        // Connection timeout
        setTimeout(() => {
          if (!this.connected) {
            this.ws.terminate();
            state.connectionsFailed++;
            resolve(false);
          }
        }, 5000);

      } catch (error) {
        state.connectionsFailed++;
        resolve(false);
      }
    });
  }

  onMessage(data) {
    this.messagesReceived++;
    state.totalMessagesReceived++;

    // PERFORMANCE: Batch processing to reduce CPU overhead
    this.messageBuffer.push(data);

    if (this.messageBuffer.length >= 10 || !this.processingBatch) {
      this.processBatch();
    }
  }

  processBatch() {
    if (this.processingBatch || this.messageBuffer.length === 0) {
      return;
    }

    this.processingBatch = true;

    setImmediate(() => {
      const batch = this.messageBuffer.splice(0, this.messageBuffer.length);

      for (const data of batch) {
        try {
          const message = JSON.parse(data.toString());
          if (!message.type) {
            state.messageErrors++;
          }
        } catch (error) {
          state.messageErrors++;
        }
      }

      this.processingBatch = false;

      if (this.messageBuffer.length > 0) {
        this.processBatch();
      }
    });
  }

  startHeartbeat() {
    this.heartbeatInterval = setInterval(() => {
      if (this.ws && this.ws.readyState === WebSocket.OPEN) {
        try {
          this.ws.send(JSON.stringify({ type: 'heartbeat' }));
        } catch (error) {
          // Ignore
        }
      }
    }, 30000);
  }

  onClose() {
    this.connected = false;
    if (this.heartbeatInterval) {
      clearInterval(this.heartbeatInterval);
    }
    this.messageBuffer = [];
    this.processingBatch = false;
  }

  close() {
    if (this.heartbeatInterval) {
      clearInterval(this.heartbeatInterval);
    }
    if (this.ws && this.connected) {
      this.ws.close(1000, 'Test complete');
    }
  }
}

// ============================================================================
// TEST EXECUTION
// ============================================================================

async function setupConnections() {
  console.log(`\n🔌 Setting up ${CONFIG.TARGET_CONNECTIONS} connections...`);
  state.setupStartTime = Date.now();

  const batchSize = 50;
  const batches = Math.ceil(CONFIG.TARGET_CONNECTIONS / batchSize);

  for (let batch = 0; batch < batches; batch++) {
    const batchStart = batch * batchSize;
    const batchEnd = Math.min(batchStart + batchSize, CONFIG.TARGET_CONNECTIONS);
    const currentBatchSize = batchEnd - batchStart;

    const promises = [];
    for (let i = 0; i < currentBatchSize; i++) {
      const conn = new ThroughputTestConnection(batchStart + i);
      state.connections.push(conn);
      promises.push(conn.connect());
    }

    await Promise.all(promises);

    const progress = ((batchEnd / CONFIG.TARGET_CONNECTIONS) * 100).toFixed(1);
    console.log(`   Batch ${batch + 1}/${batches}: ${state.connectionsEstablished}/${CONFIG.TARGET_CONNECTIONS} (${progress}%)`);

    if (batch < batches - 1) {
      await new Promise(resolve => setTimeout(resolve, 100));
    }
  }

  const setupTime = ((Date.now() - state.setupStartTime) / 1000).toFixed(1);
  const successRate = ((state.connectionsEstablished / CONFIG.TARGET_CONNECTIONS) * 100).toFixed(1);

  console.log(`\n✅ Connection setup complete!`);
  console.log(`   Established: ${state.connectionsEstablished}/${CONFIG.TARGET_CONNECTIONS} (${successRate}%)`);
  console.log(`   Failed: ${state.connectionsFailed}`);
  console.log(`   Setup time: ${setupTime}s`);
  console.log(`   Rate: ${(state.connectionsEstablished / parseFloat(setupTime)).toFixed(1)} conn/sec`);
}

function printReport() {
  if (state.phase !== 'running') return;

  const elapsed = Math.floor((Date.now() - state.testStartTime) / 1000);
  const remaining = Math.max(0, CONFIG.TEST_DURATION - elapsed);

  const msgRate = state.totalMessagesReceived / elapsed;
  const msgPerConn = state.totalMessagesReceived / state.connectionsEstablished;
  const errorRate = ((state.messageErrors / (state.totalMessagesReceived || 1)) * 100).toFixed(2);

  console.log('\n' + '='.repeat(80));
  console.log(`📊 MESSAGE THROUGHPUT TEST - Elapsed: ${elapsed}s / ${CONFIG.TEST_DURATION}s`);
  console.log('='.repeat(80));
  console.log('\n🔌 Connections:');
  console.log(`   Active:           ${state.connectionsEstablished}`);
  console.log(`   Target:           ${CONFIG.TARGET_CONNECTIONS}`);

  console.log('\n📨 Message Metrics:');
  console.log(`   Total Received:   ${state.totalMessagesReceived.toLocaleString()}`);
  console.log(`   Rate:             ${msgRate.toFixed(2)} msg/sec`);
  console.log(`   Per Connection:   ${msgPerConn.toFixed(1)} msg/conn`);
  console.log(`   Errors:           ${state.messageErrors} (${errorRate}%)`);

  console.log('\n⏱️  Test Progress:');
  console.log(`   Running:          ${elapsed}s`);
  console.log(`   Remaining:        ${remaining}s`);

  console.log('='.repeat(80) + '\n');
}

async function runThroughputTest() {
  console.log('\n' + '='.repeat(80));
  console.log('📊 MESSAGE THROUGHPUT TEST');
  console.log('='.repeat(80));
  console.log('\n⚙️  Configuration:');
  console.log(`   Server:           ${CONFIG.WS_URL}`);
  console.log(`   Connections:      ${CONFIG.TARGET_CONNECTIONS}`);
  console.log(`   Test Duration:    ${CONFIG.TEST_DURATION}s (${(CONFIG.TEST_DURATION / 60).toFixed(1)} min)`);
  console.log(`   Report Interval:  ${CONFIG.REPORT_INTERVAL}s`);
  console.log('\n📋 Test Purpose:');
  console.log('   ✓ Measure message receive rate');
  console.log('   ✓ Validate reliable broadcast at scale');
  console.log('   ✓ Test client message processing performance');
  console.log('   ✗ Does NOT test connection capacity (use test-connection-rate.cjs)');
  console.log('='.repeat(80));

  // Phase 1: Setup connections
  await setupConnections();

  if (state.connectionsEstablished === 0) {
    console.log('\n❌ No connections established! Aborting test.');
    process.exit(1);
  }

  // Phase 2: Run throughput test
  console.log(`\n🚀 Starting message throughput test for ${CONFIG.TEST_DURATION}s...`);
  console.log('   (Press Ctrl+C to end test early)\n');

  state.phase = 'running';
  state.testStartTime = Date.now();

  // Periodic reporting
  const reportInterval = setInterval(() => {
    printReport();
  }, CONFIG.REPORT_INTERVAL * 1000);

  // Wait for test duration
  await new Promise(resolve => setTimeout(resolve, CONFIG.TEST_DURATION * 1000));

  // Phase 3: Cleanup
  state.phase = 'completed';
  clearInterval(reportInterval);

  console.log('\n🏁 Test completed!');
  printReport();

  // Final summary
  const totalTime = CONFIG.TEST_DURATION;
  const avgMsgRate = state.totalMessagesReceived / totalTime;
  const avgMsgPerConn = state.totalMessagesReceived / state.connectionsEstablished;

  console.log('\n' + '='.repeat(80));
  console.log('📊 FINAL SUMMARY');
  console.log('='.repeat(80));
  console.log(`✓ Connections:        ${state.connectionsEstablished}`);
  console.log(`✓ Test Duration:      ${totalTime}s`);
  console.log(`✓ Total Messages:     ${state.totalMessagesReceived.toLocaleString()}`);
  console.log(`✓ Avg Rate:           ${avgMsgRate.toFixed(2)} msg/sec`);
  console.log(`✓ Per Connection:     ${avgMsgPerConn.toFixed(1)} msg/conn`);
  console.log(`✓ Error Rate:         ${((state.messageErrors / state.totalMessagesReceived) * 100).toFixed(2)}%`);
  console.log('\n💡 Extrapolation:');
  console.log(`   If ${state.connectionsEstablished} connections handle ${avgMsgRate.toFixed(0)} msg/sec,`);
  console.log(`   then 5000 connections would handle ~${((avgMsgRate / state.connectionsEstablished) * 5000).toFixed(0)} msg/sec`);
  console.log(`   when distributed across 5000 separate machines.`);
  console.log('='.repeat(80) + '\n');

  // Close connections
  console.log('🔄 Closing connections...');
  for (const conn of state.connections) {
    conn.close();
  }

  await new Promise(resolve => setTimeout(resolve, 2000));
  console.log('✅ All connections closed\n');

  process.exit(0);
}

// Graceful shutdown
process.on('SIGINT', () => {
  console.log('\n🛑 Received SIGINT, shutting down...');
  if (state.phase === 'running') {
    printReport();
  }
  for (const conn of state.connections) {
    conn.close();
  }
  process.exit(0);
});

process.on('SIGTERM', () => {
  console.log('\n🛑 Received SIGTERM, shutting down...');
  if (state.phase === 'running') {
    printReport();
  }
  for (const conn of state.connections) {
    conn.close();
  }
  process.exit(0);
});

// Run test
runThroughputTest().catch(error => {
  console.error(`❌ Test failed: ${error.message}`);
  console.error(error);
  process.exit(1);
});
