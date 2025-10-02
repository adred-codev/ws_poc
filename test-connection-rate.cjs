const WebSocket = require('ws');

const SERVER_URL = process.argv[2] || 'ws://localhost:3002/ws';
const TARGET_CONNECTIONS = parseInt(process.argv[3]) || 3000;
const PREFIX = SERVER_URL.includes('3002') ? 'GO' : 'NODE';

console.log(`🧪 [${PREFIX}] Testing connection success rate with ${TARGET_CONNECTIONS} connections`);
console.log(`📊 [${PREFIX}] Target: 90%+ success rate`);

let connectionAttempts = 0;
let connectionSuccesses = 0;
let connectionFailures = 0;
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
        resolve(true);

        // Close immediately after successful connection
        ws.close(1000, 'Test complete');
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

  console.log(`\n🏁 [${PREFIX}] Connection rate test completed`);
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