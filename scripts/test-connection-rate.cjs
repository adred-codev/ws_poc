const WebSocket = require('ws');

const SERVER_URL = process.argv[2] || process.env.WS_URL || 'ws://localhost:3004/ws';
const TARGET_CONNECTIONS = parseInt(process.argv[3]) || parseInt(process.env.TARGET_CONNECTIONS) || 3000;
const HOLD_DURATION = parseInt(process.argv[4]) || parseInt(process.env.DURATION) || 0; // 0 = close immediately (original behavior)
const PREFIX = SERVER_URL.includes('3002') ? 'GO' : 'NODE';

const testMode = HOLD_DURATION > 0 ? 'CAPACITY TEST (hold connections)' : 'CONNECTION RATE TEST (close immediately)';

console.log(`🧪 [${PREFIX}] ${testMode}`);
console.log(`📊 [${PREFIX}] Target: ${TARGET_CONNECTIONS} connections`);
if (HOLD_DURATION > 0) {
  console.log(`⏱️  [${PREFIX}] Hold Duration: ${HOLD_DURATION}s`);
}
console.log(`📊 [${PREFIX}] Success Target: 90%+`);

let connectionAttempts = 0;
let connectionSuccesses = 0;
let connectionFailures = 0;
let activeConnections = []; // Store connections if holding
let startTime = Date.now();

function createConnection(index) {
  return new Promise((resolve) => {
    const ws = new WebSocket(SERVER_URL);
    let resolved = false;

    const timeout = setTimeout(() => {
      if (!resolved) {
        resolved = true;
        connectionFailures++;
        resolve(false);
      }
    }, 5000); // 5 second timeout

    ws.on('open', () => {
      if (!resolved) {
        resolved = true;
        clearTimeout(timeout);
        connectionSuccesses++;

        if (HOLD_DURATION > 0) {
          // CAPACITY TEST MODE: Hold connection open
          activeConnections.push(ws);

          // Send heartbeat every 30s to prevent timeout
          const heartbeat = setInterval(() => {
            if (ws.readyState === WebSocket.OPEN) {
              ws.send(JSON.stringify({ type: 'heartbeat' }));
            } else {
              clearInterval(heartbeat);
            }
          }, 30000);

          ws.heartbeatInterval = heartbeat;
        } else {
          // CONNECTION RATE TEST MODE: Close immediately (original behavior)
          ws.close(1000, 'Test complete');
        }

        resolve(true);
      }
    });

    // Handle WebSocket pings from server (respond with pong)
    // The ws library should handle this automatically, but we'll be explicit
    ws.on('ping', () => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.pong();
      }
    });

    ws.on('error', (error) => {
      if (!resolved) {
        resolved = true;
        clearTimeout(timeout);
        connectionFailures++;
        resolve(false);
      }
    });

    ws.on('close', () => {
      clearTimeout(timeout);
    });

    connectionAttempts++;
  });
}

async function testConnectionRate() {
  const batchSize = 50;
  const batches = Math.ceil(TARGET_CONNECTIONS / batchSize);

  for (let batch = 0; batch < batches; batch++) {
    const batchStart = batch * batchSize;
    const batchEnd = Math.min(batchStart + batchSize, TARGET_CONNECTIONS);
    const currentBatchSize = batchEnd - batchStart;

    console.log(`🔄 [${PREFIX}] Batch ${batch + 1}/${batches} (${batchStart + 1}-${batchEnd})`);

    // Create batch of connections
    const promises = [];
    for (let i = 0; i < currentBatchSize; i++) {
      promises.push(createConnection(batchStart + i));
    }

    await Promise.all(promises);

    const successRate = (connectionSuccesses / connectionAttempts * 100).toFixed(1);
    console.log(`📊 [${PREFIX}] Progress: ${connectionAttempts}/${TARGET_CONNECTIONS}, Success: ${connectionSuccesses}, Failed: ${connectionFailures}, Rate: ${successRate}%`);

    // Small delay between batches
    if (batch < batches - 1) {
      await new Promise(resolve => setTimeout(resolve, 100));
    }
  }

  const totalTime = (Date.now() - startTime) / 1000;
  const finalSuccessRate = (connectionSuccesses / connectionAttempts * 100).toFixed(1);

  console.log(`\n🏁 [${PREFIX}] Connection establishment completed`);
  console.log(`📊 [${PREFIX}] Final Results:`);
  console.log(`   Total attempts: ${connectionAttempts}`);
  console.log(`   Successful connections: ${connectionSuccesses}`);
  console.log(`   Failed connections: ${connectionFailures}`);
  console.log(`   Success rate: ${finalSuccessRate}%`);
  console.log(`   Total time: ${totalTime.toFixed(1)}s`);
  console.log(`   Connections per second: ${(connectionSuccesses / totalTime).toFixed(1)}`);

  if (parseFloat(finalSuccessRate) >= 90) {
    console.log(`✅ [${PREFIX}] SUCCESS: Target 90%+ success rate achieved!`);
  } else {
    console.log(`❌ [${PREFIX}] FAILED: Success rate below 90% target`);
  }

  // If holding connections, wait for duration
  if (HOLD_DURATION > 0) {
    console.log(`\n⏳ [${PREFIX}] Holding ${activeConnections.length} connections for ${HOLD_DURATION}s...`);
    console.log(`   (Press Ctrl+C to end test early)`);

    await new Promise(resolve => setTimeout(resolve, HOLD_DURATION * 1000));

    console.log(`\n🔄 [${PREFIX}] Closing ${activeConnections.length} connections...`);
    for (const ws of activeConnections) {
      if (ws.heartbeatInterval) {
        clearInterval(ws.heartbeatInterval);
      }
      if (ws.readyState === WebSocket.OPEN) {
        ws.close(1000, 'Test complete');
      }
    }

    // Wait for connections to close
    await new Promise(resolve => setTimeout(resolve, 2000));
    console.log(`✅ [${PREFIX}] All connections closed`);
  }

  process.exit(0);
}

testConnectionRate().catch(console.error);

// Handle process interruption
process.on('SIGINT', () => {
  const finalSuccessRate = connectionAttempts > 0 ? (connectionSuccesses / connectionAttempts * 100).toFixed(1) : 0;
  console.log(`\n🛑 [${PREFIX}] Test interrupted`);
  console.log(`📊 [${PREFIX}] Partial results: ${connectionSuccesses}/${connectionAttempts} (${finalSuccessRate}%)`);
  process.exit(0);
});