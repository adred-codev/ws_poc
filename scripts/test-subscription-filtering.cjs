#!/usr/bin/env node

/**
 * Test Hierarchical Subscription Filtering
 *
 * Validates that the WebSocket server correctly filters messages based on hierarchical channel subscriptions.
 *
 * Test scenarios:
 * 1. Client subscribes to specific hierarchical channels (BTC.trade, ETH.trade)
 * 2. Server sends messages to different hierarchical channels
 * 3. Client only receives messages for subscribed event types
 * 4. Client can unsubscribe from specific event types
 *
 * Channel format: {SYMBOL}.{EVENT_TYPE}
 * - Event types: trade, liquidity, metadata, social, favorites, creation, analytics, balances
 */

const WebSocket = require('ws');
const { spawn } = require('child_process');
const path = require('path');

// Test configuration
const WS_URL = process.env.WS_URL || 'ws://localhost:3002/ws';
const NATS_URL = process.env.NATS_URL || 'nats://localhost:4222';

// ANSI color codes
const colors = {
  reset: '\x1b[0m',
  green: '\x1b[32m',
  red: '\x1b[31m',
  yellow: '\x1b[33m',
  blue: '\x1b[34m',
  cyan: '\x1b[36m',
};

function log(message, color = 'reset') {
  console.log(`${colors[color]}${message}${colors.reset}`);
}

function logSuccess(message) {
  log(`✅ ${message}`, 'green');
}

function logError(message) {
  log(`❌ ${message}`, 'red');
}

function logInfo(message) {
  log(`ℹ️  ${message}`, 'cyan');
}

function logWarning(message) {
  log(`⚠️  ${message}`, 'yellow');
}

/**
 * Create a test WebSocket client
 */
function createTestClient(name) {
  return new Promise((resolve, reject) => {
    const ws = new WebSocket(WS_URL);
    const receivedMessages = [];

    ws.on('open', () => {
      logInfo(`[${name}] Connected to ${WS_URL}`);
      resolve({
        ws,
        receivedMessages,
        name,

        subscribe(channels) {
          return new Promise((res) => {
            const msg = JSON.stringify({
              type: 'subscribe',
              data: { channels }
            });
            ws.send(msg);
            logInfo(`[${name}] Subscribed to: ${channels.join(', ')}`);

            // Wait for subscription ack
            const ackListener = (data) => {
              try {
                const response = JSON.parse(data);
                if (response.type === 'subscription_ack') {
                  logSuccess(`[${name}] Subscription confirmed: ${response.count} channels`);
                  ws.off('message', ackListener);
                  res();
                }
              } catch (e) {}
            };
            ws.on('message', ackListener);
          });
        },

        unsubscribe(channels) {
          return new Promise((res) => {
            const msg = JSON.stringify({
              type: 'unsubscribe',
              data: { channels }
            });
            ws.send(msg);
            logInfo(`[${name}] Unsubscribed from: ${channels.join(', ')}`);

            // Wait for unsubscription ack
            const ackListener = (data) => {
              try {
                const response = JSON.parse(data);
                if (response.type === 'unsubscription_ack') {
                  logSuccess(`[${name}] Unsubscription confirmed: ${response.count} channels remaining`);
                  ws.off('message', ackListener);
                  res();
                }
              } catch (e) {}
            };
            ws.on('message', ackListener);
          });
        },

        close() {
          ws.close();
          logInfo(`[${name}] Disconnected`);
        }
      });
    });

    ws.on('message', (data) => {
      try {
        const msg = JSON.parse(data);
        // Track all messages except acks
        if (msg.type !== 'subscription_ack' && msg.type !== 'unsubscription_ack' && msg.type !== 'pong') {
          receivedMessages.push(msg);
          logInfo(`[${name}] Received: ${JSON.stringify(msg).substring(0, 100)}...`);
        }
      } catch (e) {
        logWarning(`[${name}] Failed to parse message: ${e.message}`);
      }
    });

    ws.on('error', (error) => {
      logError(`[${name}] WebSocket error: ${error.message}`);
      reject(error);
    });

    ws.on('close', () => {
      logInfo(`[${name}] Connection closed`);
    });
  });
}

/**
 * Publish a message to NATS
 */
async function publishToNATS(subject, data) {
  const { connect, StringCodec } = require('nats');

  try {
    const nc = await connect({ servers: NATS_URL });
    const sc = StringCodec();

    nc.publish(subject, sc.encode(JSON.stringify(data)));
    await nc.flush();
    await nc.close();

    logSuccess(`Published to ${subject}: ${JSON.stringify(data)}`);
  } catch (error) {
    logError(`Failed to publish to NATS: ${error.message}`);
    throw error;
  }
}

/**
 * Wait for a specified duration
 */
function wait(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

/**
 * Test 1: Basic subscription filtering
 */
async function testBasicFiltering() {
  log('\n━━━ Test 1: Basic Subscription Filtering ━━━', 'blue');

  // Create two clients
  const client1 = await createTestClient('Client1');
  const client2 = await createTestClient('Client2');

  // Client 1 subscribes to BTC.trade and ETH.trade
  await client1.subscribe(['BTC.trade', 'ETH.trade']);

  // Client 2 subscribes to SOL.trade
  await client2.subscribe(['SOL.trade']);

  await wait(1000);

  // Publish to different hierarchical channels
  await publishToNATS('odin.token.BTC.trade', { symbol: 'BTC', price: 50000, type: 'trade' });
  await wait(500);
  await publishToNATS('odin.token.ETH.trade', { symbol: 'ETH', price: 3000, type: 'trade' });
  await wait(500);
  await publishToNATS('odin.token.SOL.trade', { symbol: 'SOL', price: 100, type: 'trade' });
  await wait(1000);

  // Verify results
  const client1Messages = client1.receivedMessages.length;
  const client2Messages = client2.receivedMessages.length;

  log(`\nResults:`, 'yellow');
  log(`  Client1 (BTC.trade, ETH.trade): ${client1Messages} messages`, 'yellow');
  log(`  Client2 (SOL.trade): ${client2Messages} messages`, 'yellow');

  if (client1Messages === 2 && client2Messages === 1) {
    logSuccess('Test 1 PASSED: Clients received only subscribed messages');
  } else {
    logError(`Test 1 FAILED: Expected Client1=2, Client2=1, got Client1=${client1Messages}, Client2=${client2Messages}`);
  }

  // Cleanup
  client1.close();
  client2.close();

  return client1Messages === 2 && client2Messages === 1;
}

/**
 * Test 2: Unsubscribe functionality
 */
async function testUnsubscribe() {
  log('\n━━━ Test 2: Unsubscribe Functionality ━━━', 'blue');

  const client = await createTestClient('Client');

  // Subscribe to BTC.trade and ETH.trade
  await client.subscribe(['BTC.trade', 'ETH.trade']);
  await wait(500);

  // Publish to BTC.trade - should receive
  await publishToNATS('odin.token.BTC.trade', { symbol: 'BTC', price: 50000, type: 'trade' });
  await wait(500);

  const messagesBeforeUnsub = client.receivedMessages.length;

  // Unsubscribe from BTC.trade
  await client.unsubscribe(['BTC.trade']);
  await wait(500);

  // Publish to BTC.trade again - should NOT receive
  await publishToNATS('odin.token.BTC.trade', { symbol: 'BTC', price: 51000, type: 'trade' });
  await wait(500);

  // Publish to ETH.trade - should still receive
  await publishToNATS('odin.token.ETH.trade', { symbol: 'ETH', price: 3000, type: 'trade' });
  await wait(1000);

  const messagesAfterUnsub = client.receivedMessages.length;

  log(`\nResults:`, 'yellow');
  log(`  Before unsubscribe: ${messagesBeforeUnsub} messages`, 'yellow');
  log(`  After unsubscribe: ${messagesAfterUnsub} messages`, 'yellow');

  // Should have 2 messages total: 1 BTC.trade before unsub, 1 ETH.trade after unsub
  if (messagesAfterUnsub === 2) {
    logSuccess('Test 2 PASSED: Unsubscribe stopped messages from BTC.trade');
  } else {
    logError(`Test 2 FAILED: Expected 2 messages, got ${messagesAfterUnsub}`);
  }

  client.close();

  return messagesAfterUnsub === 2;
}

/**
 * Test 3: Multiple subscriptions
 */
async function testMultipleSubscriptions() {
  log('\n━━━ Test 3: Multiple Subscriptions ━━━', 'blue');

  const client = await createTestClient('Client');

  // Subscribe to multiple hierarchical channels
  await client.subscribe(['BTC.trade', 'ETH.trade', 'SOL.trade', 'ADA.trade', 'DOT.trade']);
  await wait(500);

  // Publish to all channels
  const symbols = ['BTC', 'ETH', 'SOL', 'ADA', 'DOT'];
  for (const symbol of symbols) {
    await publishToNATS(`odin.token.${symbol}.trade`, { symbol, price: Math.random() * 1000, type: 'trade' });
    await wait(200);
  }

  await wait(1000);

  const received = client.receivedMessages.length;

  log(`\nResults:`, 'yellow');
  log(`  Subscribed to: ${symbols.length} channels`, 'yellow');
  log(`  Received: ${received} messages`, 'yellow');

  if (received === 5) {
    logSuccess('Test 3 PASSED: Received all messages from subscribed channels');
  } else {
    logError(`Test 3 FAILED: Expected 5 messages, got ${received}`);
  }

  client.close();

  return received === 5;
}

/**
 * Test 4: No subscription = no messages
 */
async function testNoSubscription() {
  log('\n━━━ Test 4: No Subscription = No Messages ━━━', 'blue');

  const client = await createTestClient('Client');

  // Don't subscribe to anything
  await wait(500);

  // Publish to various hierarchical channels
  await publishToNATS('odin.token.BTC.trade', { symbol: 'BTC', price: 50000, type: 'trade' });
  await wait(500);
  await publishToNATS('odin.token.ETH.trade', { symbol: 'ETH', price: 3000, type: 'trade' });
  await wait(1000);

  const received = client.receivedMessages.length;

  log(`\nResults:`, 'yellow');
  log(`  Subscribed to: 0 channels`, 'yellow');
  log(`  Received: ${received} messages`, 'yellow');

  if (received === 0) {
    logSuccess('Test 4 PASSED: No subscriptions = no messages received');
  } else {
    logError(`Test 4 FAILED: Expected 0 messages, got ${received}`);
  }

  client.close();

  return received === 0;
}

/**
 * Main test runner
 */
async function runTests() {
  log('╔═══════════════════════════════════════════════════════════╗', 'cyan');
  log('║     WebSocket Subscription Filtering Test Suite          ║', 'cyan');
  log('╚═══════════════════════════════════════════════════════════╝', 'cyan');

  logInfo(`WebSocket URL: ${WS_URL}`);
  logInfo(`NATS URL: ${NATS_URL}`);
  log('');

  const results = [];

  try {
    // Check if NATS package is installed
    try {
      require('nats');
    } catch (e) {
      logError('NATS package not found. Please run: npm install nats');
      process.exit(1);
    }

    results.push({ name: 'Basic Filtering', passed: await testBasicFiltering() });
    await wait(2000);

    results.push({ name: 'Unsubscribe', passed: await testUnsubscribe() });
    await wait(2000);

    results.push({ name: 'Multiple Subscriptions', passed: await testMultipleSubscriptions() });
    await wait(2000);

    results.push({ name: 'No Subscription', passed: await testNoSubscription() });

  } catch (error) {
    logError(`Test suite failed: ${error.message}`);
    console.error(error);
    process.exit(1);
  }

  // Summary
  log('\n╔═══════════════════════════════════════════════════════════╗', 'cyan');
  log('║                     Test Summary                          ║', 'cyan');
  log('╚═══════════════════════════════════════════════════════════╝', 'cyan');

  const passed = results.filter(r => r.passed).length;
  const total = results.length;

  results.forEach(result => {
    const status = result.passed ? '✅ PASS' : '❌ FAIL';
    const color = result.passed ? 'green' : 'red';
    log(`  ${status} - ${result.name}`, color);
  });

  log('');
  log(`Total: ${passed}/${total} tests passed`, passed === total ? 'green' : 'red');

  if (passed === total) {
    logSuccess('\n🎉 All tests passed! Subscription filtering is working correctly.');
    process.exit(0);
  } else {
    logError(`\n❌ ${total - passed} test(s) failed. Please check the implementation.`);
    process.exit(1);
  }
}

// Run tests
runTests().catch(error => {
  logError(`Unhandled error: ${error.message}`);
  console.error(error);
  process.exit(1);
});
