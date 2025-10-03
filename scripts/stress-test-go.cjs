const WebSocket = require('ws');

const GO_SERVER_URL = 'ws://localhost:3002/ws';
const NUM_CONNECTIONS = process.argv[2] || 50;
const DURATION_SECONDS = process.argv[3] || 30;

console.log(`🧪 [GO] Stress testing Go server with ${NUM_CONNECTIONS} connections for ${DURATION_SECONDS} seconds`);

let connections = [];
let messagesReceived = 0;
let connectionCount = 0;
let priceUpdates = 0;
let heartbeats = 0;
let pongs = 0;

// Create multiple WebSocket connections
for (let i = 0; i < NUM_CONNECTIONS; i++) {
  setTimeout(() => {
    const ws = new WebSocket(GO_SERVER_URL);

    ws.on('open', () => {
      connectionCount++;
      console.log(`✅ [GO] Connection ${connectionCount}/${NUM_CONNECTIONS} established`);

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

      // Parse message to categorize types
      try {
        const msg = JSON.parse(data.toString());
        if (msg.type === 'price:update') {
          priceUpdates++;
        } else if (msg.type === 'heartbeat') {
          heartbeats++;
        } else if (msg.type === 'pong') {
          pongs++;
        }
      } catch (e) {
        // Ignore parse errors
      }

      // Log progress every 50 messages
      if (messagesReceived % 50 === 0) {
        console.log(`📊 ${messagesReceived} total (${priceUpdates} price updates, ${heartbeats} heartbeats, ${pongs} pongs)`);
      }
    });

    ws.on('error', (error) => {
      console.error(`❌ [GO] Connection ${i} error:`, error.message);
    });

    ws.on('close', () => {
      if (ws.pingInterval) {
        clearInterval(ws.pingInterval);
      }
    });

    connections.push(ws);
  }, i * 100); // Stagger connections by 100ms
}

// Real-time stats every 5 seconds
const statsInterval = setInterval(() => {
  if (connectionCount > 0) {
    console.log(`⏱️  [${Math.floor(Date.now()/1000) % 1000}s] ${messagesReceived} messages (${priceUpdates} price, ${heartbeats} heartbeat) across ${connectionCount} connections`);
  }
}, 5000);

// Stop test after duration
setTimeout(() => {
  clearInterval(statsInterval);

  console.log(`\n🏁 [GO] Test completed after ${DURATION_SECONDS} seconds`);
  console.log(`📊 [GO] Final stats:`);
  console.log(`   Server: Go WebSocket (ws://localhost:3002/ws)`);
  console.log(`   Connections created: ${connectionCount}`);
  console.log(`   Total messages received: ${messagesReceived}`);
  console.log(`   Price updates: ${priceUpdates}`);
  console.log(`   Heartbeats: ${heartbeats}`);
  console.log(`   Pongs: ${pongs}`);
  console.log(`   Messages per connection: ${(messagesReceived / connectionCount).toFixed(1)}`);
  console.log(`   Messages per second: ${(messagesReceived / DURATION_SECONDS).toFixed(1)}`);

  // Close all connections
  connections.forEach(ws => {
    if (ws.readyState === WebSocket.OPEN) {
      ws.close();
    }
  });

  process.exit(0);
}, DURATION_SECONDS * 1000);