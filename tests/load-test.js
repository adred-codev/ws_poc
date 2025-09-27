import WebSocket from 'ws';
import { performance } from 'perf_hooks';

class LoadTester {
  constructor() {
    this.connections = [];
    this.stats = {
      totalConnections: 0,
      successfulConnections: 0,
      failedConnections: 0,
      totalMessages: 0,
      totalLatency: 0,
      minLatency: Infinity,
      maxLatency: 0,
      startTime: 0,
      endTime: 0,
      errors: []
    };
    this.isRunning = false;
  }

  async runTest(config = {}) {
    const {
      maxConnections = 100,
      connectionRate = 10, // connections per second
      testDuration = 60000, // 1 minute
      serverUrl = 'ws://localhost:3000/ws',
      pingInterval = 5000 // 5 seconds
    } = config;

    console.log('🚀 Starting WebSocket Load Test');
    console.log(`   Target Connections: ${maxConnections}`);
    console.log(`   Connection Rate: ${connectionRate}/sec`);
    console.log(`   Test Duration: ${testDuration/1000}s`);
    console.log(`   Server URL: ${serverUrl}\n`);

    this.stats.startTime = performance.now();
    this.isRunning = true;

    try {
      // Create connections progressively
      await this.createConnections(maxConnections, connectionRate, serverUrl, pingInterval);

      // Wait for test duration
      console.log(`⏳ Running test for ${testDuration/1000} seconds...`);
      await this.sleep(testDuration);

      // Clean up
      await this.cleanup();

    } catch (error) {
      console.error('❌ Test failed:', error);
      this.stats.errors.push(error.message);
    } finally {
      this.isRunning = false;
      this.stats.endTime = performance.now();
      this.printResults();
    }
  }

  async createConnections(maxConnections, connectionRate, serverUrl, pingInterval) {
    const intervalMs = 1000 / connectionRate;

    for (let i = 0; i < maxConnections && this.isRunning; i++) {
      try {
        await this.createConnection(i, serverUrl, pingInterval);
        this.stats.totalConnections++;

        // Progress indicator
        if ((i + 1) % 50 === 0 || i === maxConnections - 1) {
          console.log(`📈 Created ${i + 1}/${maxConnections} connections (${this.stats.successfulConnections} active)`);
        }

        // Rate limiting
        if (i < maxConnections - 1) {
          await this.sleep(intervalMs);
        }

      } catch (error) {
        console.error(`❌ Failed to create connection ${i}:`, error.message);
        this.stats.failedConnections++;
        this.stats.errors.push(`Connection ${i}: ${error.message}`);
      }
    }

    console.log(`✅ Connection creation complete: ${this.stats.successfulConnections}/${maxConnections} successful\n`);
  }

  async createConnection(id, serverUrl, pingInterval) {
    return new Promise((resolve, reject) => {
      const connectStart = performance.now();
      const ws = new WebSocket(serverUrl);

      const connection = {
        id,
        ws,
        connected: false,
        messageCount: 0,
        lastPing: 0,
        pingInterval: null,
        latencies: []
      };

      // Connection timeout
      const timeout = setTimeout(() => {
        if (!connection.connected) {
          ws.terminate();
          reject(new Error(`Connection ${id} timeout`));
        }
      }, 5000);

      ws.on('open', () => {
        clearTimeout(timeout);
        const connectTime = performance.now() - connectStart;

        connection.connected = true;
        this.stats.successfulConnections++;
        this.connections.push(connection);

        // Start periodic ping
        connection.pingInterval = setInterval(() => {
          this.sendPing(connection);
        }, pingInterval);

        resolve();
      });

      ws.on('message', (data) => {
        try {
          const message = JSON.parse(data.toString());
          connection.messageCount++;
          this.stats.totalMessages++;

          // Handle pong for latency measurement
          if (message.type === 'pong' && connection.lastPing) {
            const latency = performance.now() - connection.lastPing;
            connection.latencies.push(latency);
            this.updateLatencyStats(latency);
          }

        } catch (error) {
          // Ignore parsing errors for this test
        }
      });

      ws.on('close', (code, reason) => {
        connection.connected = false;
        if (connection.pingInterval) {
          clearInterval(connection.pingInterval);
        }
        this.stats.successfulConnections--;
      });

      ws.on('error', (error) => {
        clearTimeout(timeout);
        connection.connected = false;
        if (connection.pingInterval) {
          clearInterval(connection.pingInterval);
        }
        reject(error);
      });
    });
  }

  sendPing(connection) {
    if (connection.connected && connection.ws.readyState === WebSocket.OPEN) {
      connection.lastPing = performance.now();
      connection.ws.send(JSON.stringify({
        type: 'ping',
        timestamp: connection.lastPing
      }));
    }
  }

  updateLatencyStats(latency) {
    this.stats.totalLatency += latency;
    this.stats.minLatency = Math.min(this.stats.minLatency, latency);
    this.stats.maxLatency = Math.max(this.stats.maxLatency, latency);
  }

  async cleanup() {
    console.log('🧹 Cleaning up connections...');

    const closePromises = this.connections.map(connection => {
      return new Promise((resolve) => {
        if (connection.pingInterval) {
          clearInterval(connection.pingInterval);
        }

        if (connection.ws.readyState === WebSocket.OPEN) {
          connection.ws.close(1000, 'Test complete');
          connection.ws.on('close', () => resolve());
        } else {
          resolve();
        }
      });
    });

    await Promise.all(closePromises);
    this.connections = [];
    console.log('✅ Cleanup complete\n');
  }

  printResults() {
    const duration = (this.stats.endTime - this.stats.startTime) / 1000;
    const avgLatency = this.stats.totalLatency > 0 ?
      (this.stats.totalLatency / this.getLatencyCount()) : 0;

    console.log('📊 LOAD TEST RESULTS');
    console.log('═'.repeat(50));
    console.log(`Test Duration:           ${duration.toFixed(1)}s`);
    console.log(`Total Connections:       ${this.stats.totalConnections}`);
    console.log(`Successful Connections:  ${this.stats.successfulConnections}`);
    console.log(`Failed Connections:      ${this.stats.failedConnections}`);
    console.log(`Success Rate:           ${((this.stats.successfulConnections / this.stats.totalConnections) * 100).toFixed(1)}%`);
    console.log(`Total Messages:         ${this.stats.totalMessages}`);
    console.log(`Messages/Second:        ${(this.stats.totalMessages / duration).toFixed(1)}`);

    if (avgLatency > 0) {
      console.log(`\n📡 LATENCY STATISTICS`);
      console.log(`Average Latency:        ${avgLatency.toFixed(1)}ms`);
      console.log(`Min Latency:            ${this.stats.minLatency.toFixed(1)}ms`);
      console.log(`Max Latency:            ${this.stats.maxLatency.toFixed(1)}ms`);
    }

    if (this.stats.errors.length > 0) {
      console.log(`\n❌ ERRORS (${this.stats.errors.length})`);
      this.stats.errors.slice(0, 10).forEach(error => {
        console.log(`   ${error}`);
      });
      if (this.stats.errors.length > 10) {
        console.log(`   ... and ${this.stats.errors.length - 10} more`);
      }
    }

    console.log('\n🎯 PERFORMANCE ANALYSIS');
    this.analyzePerformance();
  }

  analyzePerformance() {
    const successRate = (this.stats.successfulConnections / this.stats.totalConnections) * 100;
    const avgLatency = this.stats.totalLatency > 0 ?
      (this.stats.totalLatency / this.getLatencyCount()) : 0;

    // Performance ratings
    if (successRate >= 99) {
      console.log('✅ Connection Success Rate: EXCELLENT');
    } else if (successRate >= 95) {
      console.log('⚠️  Connection Success Rate: GOOD');
    } else {
      console.log('❌ Connection Success Rate: POOR');
    }

    if (avgLatency < 50) {
      console.log('✅ Average Latency: EXCELLENT');
    } else if (avgLatency < 100) {
      console.log('⚠️  Average Latency: GOOD');
    } else {
      console.log('❌ Average Latency: POOR');
    }

    if (this.stats.failedConnections === 0) {
      console.log('✅ Error Rate: EXCELLENT');
    } else if (this.stats.failedConnections < 5) {
      console.log('⚠️  Error Rate: ACCEPTABLE');
    } else {
      console.log('❌ Error Rate: HIGH');
    }

    // Recommendations
    console.log('\n💡 RECOMMENDATIONS:');
    if (successRate < 95) {
      console.log('   • Consider reducing connection rate or increasing server capacity');
    }
    if (avgLatency > 100) {
      console.log('   • Check network latency and server processing time');
    }
    if (this.stats.failedConnections > 10) {
      console.log('   • Investigate connection failures and server stability');
    }
    if (successRate >= 99 && avgLatency < 50) {
      console.log('   • Performance is excellent! Ready for production scaling');
    }
  }

  getLatencyCount() {
    return this.connections.reduce((count, conn) => count + conn.latencies.length, 0);
  }

  sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
  }
}

// Progressive test scenarios
const testScenarios = [
  {
    name: 'Light Load (100 connections)',
    config: {
      maxConnections: 100,
      connectionRate: 20,
      testDuration: 30000,
      pingInterval: 5000
    }
  },
  {
    name: 'Medium Load (500 connections)',
    config: {
      maxConnections: 500,
      connectionRate: 25,
      testDuration: 45000,
      pingInterval: 5000
    }
  },
  {
    name: 'Heavy Load (1000 connections)',
    config: {
      maxConnections: 1000,
      connectionRate: 30,
      testDuration: 60000,
      pingInterval: 10000
    }
  }
];

async function runProgressiveTest() {
  console.log('🔥 PROGRESSIVE LOAD TEST SUITE');
  console.log('══════════════════════════════\n');

  for (const scenario of testScenarios) {
    console.log(`\n🎯 ${scenario.name.toUpperCase()}`);
    console.log('─'.repeat(30));

    const tester = new LoadTester();
    await tester.runTest(scenario.config);

    console.log('\n⏸️  Waiting 10 seconds before next test...');
    await new Promise(resolve => setTimeout(resolve, 10000));
  }

  console.log('\n🏁 ALL TESTS COMPLETE!');
}

// Run single test or progressive test based on command line args
const testType = process.argv[2] || 'single';

if (testType === 'progressive') {
  runProgressiveTest().catch(console.error);
} else {
  // Single test with custom parameters
  const maxConnections = parseInt(process.argv[3]) || 100;
  const connectionRate = parseInt(process.argv[4]) || 10;
  const testDuration = parseInt(process.argv[5]) || 30000;

  const tester = new LoadTester();
  tester.runTest({
    maxConnections,
    connectionRate,
    testDuration
  }).catch(console.error);
}

// Usage examples:
// node tests/load-test.js                           # 100 connections, default settings
// node tests/load-test.js single 500 20 45000      # 500 connections, 20/sec, 45s
// node tests/load-test.js progressive               # Run all test scenarios