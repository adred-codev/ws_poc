const WebSocket = require('ws');

const GO_SERVER_URL = 'ws://localhost:3002/ws';      // Original go-server
const GO_SERVER_2_URL = 'ws://localhost:3004/ws';    // go-server-2 (optimized)
const NODE_SERVER_URL = 'ws://localhost:3001/ws';
const TARGET_CONNECTIONS = process.argv[2] || 1000;
const DURATION_SECONDS = process.argv[3] || 30;
const SERVER_TYPE = process.argv[4] || 'go'; // 'go', 'go2', or 'node'
const BATCH_SIZE = 100; // Create connections in batches
const BATCH_DELAY = 100; // ms between batches

// Use WS_URL environment variable if provided (for remote testing), otherwise use local URLs
const SERVER_URL = process.env.WS_URL ||
                   (SERVER_TYPE === 'go2' ? GO_SERVER_2_URL :
                   SERVER_TYPE === 'go' ? GO_SERVER_URL :
                   NODE_SERVER_URL);
const PREFIX = SERVER_TYPE.toUpperCase();

console.log(`🧪 [${PREFIX}] High-load stress testing with ${TARGET_CONNECTIONS} connections for ${DURATION_SECONDS} seconds`);
console.log(`📊 [${PREFIX}] Batching: ${BATCH_SIZE} connections per batch, ${BATCH_DELAY}ms delay between batches`);

let connections = [];
let messagesReceived = 0;
let connectionCount = 0;
let connectionErrors = 0;
let connectionSuccesses = 0;

// Connection statistics
let connectionStats = {
  established: 0,
  failed: 0,
  closed: 0,
  errors: new Map()
};

function createConnectionBatch(startIndex, batchSize) {
  return new Promise((resolve) => {
    let batchCompleted = 0;

    for (let i = 0; i < batchSize && (startIndex + i) < TARGET_CONNECTIONS; i++) {
      const connectionIndex = startIndex + i;

      setTimeout(() => {
        const ws = new WebSocket(SERVER_URL);

        // Set shorter timeouts for high load
        ws.on('open', () => {
          connectionCount++;
          connectionStats.established++;

          if (connectionCount % 100 === 0 || connectionCount <= 10) {
            console.log(`✅ [${PREFIX}] Connection ${connectionCount}/${TARGET_CONNECTIONS} established (${((connectionCount/TARGET_CONNECTIONS)*100).toFixed(1)}%)`);
          }

          // Reduce ping frequency for high load
          const pingInterval = setInterval(() => {
            if (ws.readyState === WebSocket.OPEN) {
              ws.send(JSON.stringify({
                type: 'ping',
                timestamp: Date.now(),
                connectionId: connectionIndex
              }));
            }
          }, 10000); // 10 second intervals instead of 5

          ws.pingInterval = pingInterval;
        });

        ws.on('message', (data) => {
          messagesReceived++;
          if (messagesReceived % 500 === 0) {
            console.log(`📊 [${PREFIX}] Received ${messagesReceived} messages across ${connectionCount} connections`);
          }
        });

        ws.on('error', (error) => {
          connectionErrors++;
          connectionStats.failed++;

          const errorType = error.code || error.message.split(' ')[0];
          connectionStats.errors.set(errorType, (connectionStats.errors.get(errorType) || 0) + 1);

          if (connectionErrors % 50 === 0 || connectionErrors <= 10) {
            console.error(`❌ [${PREFIX}] Connection ${connectionIndex} error: ${error.message}`);
          }
        });

        ws.on('close', (code, reason) => {
          connectionStats.closed++;
          if (ws.pingInterval) {
            clearInterval(ws.pingInterval);
          }
        });

        connections.push(ws);
        batchCompleted++;

        if (batchCompleted === batchSize || (startIndex + batchCompleted) === TARGET_CONNECTIONS) {
          resolve();
        }
      }, i * 10); // Stagger within batch by 10ms
    }
  });
}

async function createAllConnections() {
  const totalBatches = Math.ceil(TARGET_CONNECTIONS / BATCH_SIZE);

  for (let batch = 0; batch < totalBatches; batch++) {
    const startIndex = batch * BATCH_SIZE;
    const currentBatchSize = Math.min(BATCH_SIZE, TARGET_CONNECTIONS - startIndex);

    console.log(`🔄 [${PREFIX}] Creating batch ${batch + 1}/${totalBatches} (connections ${startIndex + 1}-${startIndex + currentBatchSize})`);

    await createConnectionBatch(startIndex, currentBatchSize);

    // Wait between batches to prevent overwhelming the server
    if (batch < totalBatches - 1) {
      await new Promise(resolve => setTimeout(resolve, BATCH_DELAY));
    }
  }

  console.log(`✅ [${PREFIX}] All connection attempts completed`);
  console.log(`📊 [${PREFIX}] Success: ${connectionStats.established}, Failed: ${connectionStats.failed}`);
}

// Progress monitoring
const progressInterval = setInterval(() => {
  const successRate = connectionStats.established > 0 ? (connectionStats.established / (connectionStats.established + connectionStats.failed) * 100).toFixed(1) : 0;
  console.log(`📈 [${PREFIX}] Progress: ${connectionCount}/${TARGET_CONNECTIONS} connections (${successRate}% success rate), ${messagesReceived} messages`);

  if (connectionStats.errors.size > 0) {
    console.log(`⚠️  [${PREFIX}] Error breakdown:`, Object.fromEntries(connectionStats.errors));
  }
}, 5000);

// Start creating connections
createAllConnections().catch(console.error);

// Stop test after duration
setTimeout(() => {
  clearInterval(progressInterval);

  console.log(`\n🏁 [${PREFIX}] Test completed after ${DURATION_SECONDS} seconds`);
  console.log(`📊 [${PREFIX}] Final stats:`);
  console.log(`   Server: ${SERVER_TYPE.charAt(0).toUpperCase() + SERVER_TYPE.slice(1)} WebSocket (${SERVER_URL})`);
  console.log(`   Target connections: ${TARGET_CONNECTIONS}`);
  console.log(`   Successful connections: ${connectionStats.established}`);
  console.log(`   Failed connections: ${connectionStats.failed}`);
  console.log(`   Success rate: ${connectionStats.established > 0 ? (connectionStats.established / TARGET_CONNECTIONS * 100).toFixed(1) : 0}%`);
  console.log(`   Messages received: ${messagesReceived}`);
  console.log(`   Messages per connection: ${connectionStats.established > 0 ? (messagesReceived / connectionStats.established).toFixed(1) : 0}`);
  console.log(`   Messages per second: ${(messagesReceived / DURATION_SECONDS).toFixed(1)}`);

  if (connectionStats.errors.size > 0) {
    console.log(`\n🚨 [${PREFIX}] Error breakdown:`);
    for (const [errorType, count] of connectionStats.errors.entries()) {
      console.log(`   ${errorType}: ${count} occurrences`);
    }
  }

  // Close all connections gracefully
  console.log(`\n🔄 [${PREFIX}] Closing ${connections.length} connections...`);
  connections.forEach((ws, index) => {
    if (ws.readyState === WebSocket.OPEN) {
      ws.close(1000, 'Test completed');
    }
  });

  setTimeout(() => {
    process.exit(0);
  }, 1000); // Give connections time to close gracefully
}, DURATION_SECONDS * 1000);

// Handle process signals gracefully
process.on('SIGINT', () => {
  console.log(`\n🛑 [${PREFIX}] Received SIGINT, gracefully shutting down...`);
  clearInterval(progressInterval);
  connections.forEach(ws => {
    if (ws.readyState === WebSocket.OPEN) {
      ws.close(1000, 'Process interrupted');
    }
  });
  process.exit(0);
});