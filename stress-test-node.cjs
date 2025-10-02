const WebSocket = require('ws');

const NODE_SERVER_URL = 'ws://localhost:3001/ws';
const NUM_CONNECTIONS = process.argv[2] || 50;
const DURATION_SECONDS = process.argv[3] || 30;

console.log(`🧪 [NODE] Stress testing Node.js server with ${NUM_CONNECTIONS} connections for ${DURATION_SECONDS} seconds`);

let connections = [];
let messagesReceived = 0;
let connectionCount = 0;

// Create multiple WebSocket connections
for (let i = 0; i < NUM_CONNECTIONS; i++) {
  setTimeout(() => {
    const ws = new WebSocket(NODE_SERVER_URL);

    ws.on('open', () => {
      connectionCount++;
      console.log(`✅ [NODE] Connection ${connectionCount}/${NUM_CONNECTIONS} established`);

      // Send periodic pings
      const pingInterval = setInterval(() => {
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(JSON.stringify({
            type: 'ping',
            timestamp: Date.now(),
            connectionId: i
          }));
        }
      }, 5000);

      ws.pingInterval = pingInterval;
    });

    ws.on('message', (data) => {
      messagesReceived++;
      if (messagesReceived % 100 === 0) {
        console.log(`📊 Received ${messagesReceived} messages across ${connectionCount} connections`);
      }
    });

    ws.on('error', (error) => {
      console.error(`❌ [NODE] Connection ${i} error:`, error.message);
    });

    ws.on('close', () => {
      if (ws.pingInterval) {
        clearInterval(ws.pingInterval);
      }
    });

    connections.push(ws);
  }, i * 100); // Stagger connections by 100ms
}

// Stop test after duration
setTimeout(() => {
  console.log(`\n🏁 [NODE] Test completed after ${DURATION_SECONDS} seconds`);
  console.log(`📊 [NODE] Final stats:`);
  console.log(`   Server: Node.js WebSocket (ws://localhost:3001/ws)`);
  console.log(`   Connections created: ${connectionCount}`);
  console.log(`   Messages received: ${messagesReceived}`);
  console.log(`   Messages per connection: ${(messagesReceived / connectionCount).toFixed(1)}`);

  // Close all connections
  connections.forEach(ws => {
    if (ws.readyState === WebSocket.OPEN) {
      ws.close();
    }
  });

  process.exit(0);
}, DURATION_SECONDS * 1000);