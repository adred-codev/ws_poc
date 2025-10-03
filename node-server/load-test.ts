import WebSocket from 'ws';
import { performance } from 'perf_hooks';

interface LoadTestConfig {
  serverUrl: string;
  maxConnections: number;
  rampUpDurationMs: number;
  testDurationMs: number;
  messageFrequencyMs: number;
  authToken?: string;
}

interface TestMetrics {
  totalConnections: number;
  successfulConnections: number;
  failedConnections: number;
  totalMessagesSent: number;
  totalMessagesReceived: number;
  averageLatency: number;
  minLatency: number;
  maxLatency: number;
  connectionsPerSecond: number;
  messagesPerSecond: number;
  errors: string[];
  startTime: number;
  endTime: number;
}

interface ClientConnection {
  id: string;
  ws: WebSocket;
  connectedAt: number;
  messagesSent: number;
  messagesReceived: number;
  latencies: number[];
  lastPingTime?: number;
}

class WebSocketLoadTester {
  private config: LoadTestConfig;
  private clients: Map<string, ClientConnection> = new Map();
  private metrics: TestMetrics;
  private isRunning = false;
  private connectionInterval?: NodeJS.Timeout;
  private messageInterval?: NodeJS.Timeout;
  private metricsInterval?: NodeJS.Timeout;

  constructor(config: LoadTestConfig) {
    this.config = config;
    this.metrics = this.initializeMetrics();
  }

  private initializeMetrics(): TestMetrics {
    return {
      totalConnections: 0,
      successfulConnections: 0,
      failedConnections: 0,
      totalMessagesSent: 0,
      totalMessagesReceived: 0,
      averageLatency: 0,
      minLatency: Infinity,
      maxLatency: 0,
      connectionsPerSecond: 0,
      messagesPerSecond: 0,
      errors: [],
      startTime: 0,
      endTime: 0
    };
  }

  async runTest(): Promise<TestMetrics> {
    console.log('🚀 Starting WebSocket Load Test');
    console.log(`   Target: ${this.config.serverUrl}`);
    console.log(`   Max Connections: ${this.config.maxConnections}`);
    console.log(`   Ramp-up Duration: ${this.config.rampUpDurationMs}ms`);
    console.log(`   Test Duration: ${this.config.testDurationMs}ms`);
    console.log('');

    this.isRunning = true;
    this.metrics.startTime = performance.now();

    // Start metrics reporting
    this.startMetricsReporting();

    // Ramp up connections
    await this.rampUpConnections();

    // Run test for specified duration
    console.log('📊 Running sustained load test...');
    await new Promise(resolve => setTimeout(resolve, this.config.testDurationMs));

    // Clean up
    await this.cleanup();

    this.metrics.endTime = performance.now();
    this.calculateFinalMetrics();

    return this.metrics;
  }

  private async rampUpConnections(): Promise<void> {
    const connectionInterval = this.config.rampUpDurationMs / this.config.maxConnections;

    console.log(`📈 Ramping up ${this.config.maxConnections} connections over ${this.config.rampUpDurationMs}ms`);
    console.log(`   Connection interval: ${connectionInterval.toFixed(1)}ms`);

    return new Promise((resolve) => {
      let connectionsCreated = 0;

      this.connectionInterval = setInterval(() => {
        if (connectionsCreated >= this.config.maxConnections || !this.isRunning) {
          if (this.connectionInterval) {
            clearInterval(this.connectionInterval);
          }
          resolve();
          return;
        }

        this.createConnection(connectionsCreated + 1);
        connectionsCreated++;
      }, connectionInterval);
    });
  }

  private createConnection(index: number): void {
    const clientId = `load-test-${index}-${Date.now()}`;
    const wsUrl = this.config.authToken
      ? `${this.config.serverUrl}?token=${this.config.authToken}`
      : this.config.serverUrl;

    try {
      const ws = new WebSocket(wsUrl);

      const client: ClientConnection = {
        id: clientId,
        ws,
        connectedAt: 0,
        messagesSent: 0,
        messagesReceived: 0,
        latencies: []
      };

      this.metrics.totalConnections++;

      ws.on('open', () => {
        client.connectedAt = performance.now();
        this.clients.set(clientId, client);
        this.metrics.successfulConnections++;

        // Start sending periodic messages
        this.startClientMessaging(client);
      });

      ws.on('message', (data: Buffer) => {
        this.handleMessage(client, data);
      });

      ws.on('close', (code: number, reason: Buffer) => {
        this.clients.delete(clientId);
        if (code !== 1000 && this.isRunning) {
          this.metrics.errors.push(`Client ${clientId} closed unexpectedly: ${code} - ${reason.toString()}`);
        }
      });

      ws.on('error', (error: Error) => {
        this.metrics.failedConnections++;
        this.metrics.errors.push(`Connection error for ${clientId}: ${error.message}`);
      });

    } catch (error) {
      this.metrics.failedConnections++;
      this.metrics.errors.push(`Failed to create connection ${clientId}: ${error}`);
    }
  }

  private startClientMessaging(client: ClientConnection): void {
    const sendMessage = () => {
      if (!this.isRunning || client.ws.readyState !== WebSocket.OPEN) {
        return;
      }

      const pingMessage = {
        type: 'ping',
        timestamp: performance.now(),
        clientId: client.id
      };

      client.lastPingTime = pingMessage.timestamp;
      client.ws.send(JSON.stringify(pingMessage));
      client.messagesSent++;
      this.metrics.totalMessagesSent++;

      // Schedule next message
      setTimeout(sendMessage, this.config.messageFrequencyMs);
    };

    // Start sending messages after a random delay to spread the load
    const initialDelay = Math.random() * this.config.messageFrequencyMs;
    setTimeout(sendMessage, initialDelay);
  }

  private handleMessage(client: ClientConnection, data: Buffer): void {
    try {
      const message = JSON.parse(data.toString());
      client.messagesReceived++;
      this.metrics.totalMessagesReceived++;

      // Calculate latency for ping/pong messages
      if (message.type === 'pong' && client.lastPingTime && message.originalTimestamp) {
        const latency = performance.now() - message.originalTimestamp;
        client.latencies.push(latency);

        this.updateLatencyMetrics(latency);
      }

    } catch (error) {
      this.metrics.errors.push(`Failed to parse message from ${client.id}: ${error}`);
    }
  }

  private updateLatencyMetrics(latency: number): void {
    this.metrics.minLatency = Math.min(this.metrics.minLatency, latency);
    this.metrics.maxLatency = Math.max(this.metrics.maxLatency, latency);
  }

  private startMetricsReporting(): void {
    let lastMessageCount = 0;
    let lastConnectionCount = 0;
    let lastTime = performance.now();

    this.metricsInterval = setInterval(() => {
      const currentTime = performance.now();
      const timeDelta = (currentTime - lastTime) / 1000; // Convert to seconds

      const currentMessageCount = this.metrics.totalMessagesReceived;
      const currentConnectionCount = this.metrics.successfulConnections;

      this.metrics.messagesPerSecond = (currentMessageCount - lastMessageCount) / timeDelta;
      this.metrics.connectionsPerSecond = (currentConnectionCount - lastConnectionCount) / timeDelta;

      console.log(`📊 Live Metrics:`);
      console.log(`   Active Connections: ${this.clients.size}/${this.config.maxConnections}`);
      console.log(`   Successful: ${this.metrics.successfulConnections}, Failed: ${this.metrics.failedConnections}`);
      console.log(`   Messages: ${this.metrics.totalMessagesSent} sent, ${this.metrics.totalMessagesReceived} received`);
      console.log(`   Rate: ${this.metrics.messagesPerSecond.toFixed(1)} msg/sec, ${this.metrics.connectionsPerSecond.toFixed(1)} conn/sec`);
      console.log(`   Latency: min=${this.metrics.minLatency.toFixed(1)}ms, max=${this.metrics.maxLatency.toFixed(1)}ms`);
      console.log(`   Errors: ${this.metrics.errors.length}`);
      console.log('');

      lastMessageCount = currentMessageCount;
      lastConnectionCount = currentConnectionCount;
      lastTime = currentTime;
    }, 5000); // Report every 5 seconds
  }

  private calculateFinalMetrics(): void {
    // Calculate average latency
    const allLatencies: number[] = [];
    for (const client of this.clients.values()) {
      allLatencies.push(...client.latencies);
    }

    if (allLatencies.length > 0) {
      this.metrics.averageLatency = allLatencies.reduce((sum, lat) => sum + lat, 0) / allLatencies.length;
    }

    // Fix infinity values
    if (this.metrics.minLatency === Infinity) {
      this.metrics.minLatency = 0;
    }
  }

  private async cleanup(): Promise<void> {
    console.log('🧹 Cleaning up connections...');
    this.isRunning = false;

    // Clear intervals
    if (this.connectionInterval) clearInterval(this.connectionInterval);
    if (this.messageInterval) clearInterval(this.messageInterval);
    if (this.metricsInterval) clearInterval(this.metricsInterval);

    // Close all WebSocket connections
    const closePromises: Promise<void>[] = [];
    for (const client of this.clients.values()) {
      closePromises.push(new Promise((resolve) => {
        if (client.ws.readyState === WebSocket.OPEN) {
          client.ws.close(1000, 'Load test complete');
          client.ws.on('close', () => resolve());
        } else {
          resolve();
        }
      }));
    }

    await Promise.all(closePromises);
    this.clients.clear();
  }

  printFinalReport(): void {
    const duration = (this.metrics.endTime - this.metrics.startTime) / 1000;

    console.log('');
    console.log('🎯 LOAD TEST RESULTS');
    console.log('=' .repeat(50));
    console.log(`Test Duration: ${duration.toFixed(1)}s`);
    console.log('');

    console.log('📊 Connection Metrics:');
    console.log(`   Total Attempted: ${this.metrics.totalConnections}`);
    console.log(`   Successful: ${this.metrics.successfulConnections} (${((this.metrics.successfulConnections / this.metrics.totalConnections) * 100).toFixed(1)}%)`);
    console.log(`   Failed: ${this.metrics.failedConnections}`);
    console.log('');

    console.log('💬 Message Metrics:');
    console.log(`   Messages Sent: ${this.metrics.totalMessagesSent}`);
    console.log(`   Messages Received: ${this.metrics.totalMessagesReceived}`);
    console.log(`   Average Rate: ${(this.metrics.totalMessagesReceived / duration).toFixed(1)} msg/sec`);
    console.log('');

    console.log('⚡ Latency Metrics:');
    console.log(`   Average: ${this.metrics.averageLatency.toFixed(1)}ms`);
    console.log(`   Minimum: ${this.metrics.minLatency.toFixed(1)}ms`);
    console.log(`   Maximum: ${this.metrics.maxLatency.toFixed(1)}ms`);
    console.log('');

    if (this.metrics.errors.length > 0) {
      console.log('❌ Errors:');
      this.metrics.errors.slice(0, 10).forEach(error => {
        console.log(`   ${error}`);
      });
      if (this.metrics.errors.length > 10) {
        console.log(`   ... and ${this.metrics.errors.length - 10} more errors`);
      }
    }
  }
}

// Test scenarios
export class LoadTestScenarios {
  static async runQuickTest(serverUrl: string, authToken?: string): Promise<TestMetrics> {
    console.log('🧪 Running Quick Test (100 connections)');
    const tester = new WebSocketLoadTester({
      serverUrl,
      maxConnections: 100,
      rampUpDurationMs: 10000, // 10 seconds
      testDurationMs: 30000,   // 30 seconds
      messageFrequencyMs: 5000, // 5 seconds
      authToken
    });

    const results = await tester.runTest();
    tester.printFinalReport();
    return results;
  }

  static async runMediumTest(serverUrl: string, authToken?: string): Promise<TestMetrics> {
    console.log('🧪 Running Medium Test (1000 connections)');
    const tester = new WebSocketLoadTester({
      serverUrl,
      maxConnections: 1000,
      rampUpDurationMs: 30000,  // 30 seconds
      testDurationMs: 120000,   // 2 minutes
      messageFrequencyMs: 10000, // 10 seconds
      authToken
    });

    const results = await tester.runTest();
    tester.printFinalReport();
    return results;
  }

  static async runStressTest(serverUrl: string, authToken?: string): Promise<TestMetrics> {
    console.log('🧪 Running Stress Test (5000 connections)');
    const tester = new WebSocketLoadTester({
      serverUrl,
      maxConnections: 5000,
      rampUpDurationMs: 60000,  // 1 minute
      testDurationMs: 300000,   // 5 minutes
      messageFrequencyMs: 15000, // 15 seconds
      authToken
    });

    const results = await tester.runTest();
    tester.printFinalReport();
    return results;
  }

  static async runProgressiveTest(serverUrl: string, authToken?: string): Promise<TestMetrics[]> {
    console.log('🎯 Running Progressive Load Test (100 → 1K → 5K)');

    const results: TestMetrics[] = [];

    // Quick test
    console.log('\n' + '='.repeat(60));
    results.push(await this.runQuickTest(serverUrl, authToken));

    // Wait between tests
    console.log('⏳ Waiting 30 seconds before next test...');
    await new Promise(resolve => setTimeout(resolve, 30000));

    // Medium test
    console.log('\n' + '='.repeat(60));
    results.push(await this.runMediumTest(serverUrl, authToken));

    // Wait between tests
    console.log('⏳ Waiting 60 seconds before stress test...');
    await new Promise(resolve => setTimeout(resolve, 60000));

    // Stress test
    console.log('\n' + '='.repeat(60));
    results.push(await this.runStressTest(serverUrl, authToken));

    // Summary
    console.log('\n' + '='.repeat(60));
    console.log('📋 PROGRESSIVE TEST SUMMARY');
    console.log('='.repeat(60));

    results.forEach((result, index) => {
      const testNames = ['Quick (100)', 'Medium (1K)', 'Stress (5K)'];
      const successRate = ((result.successfulConnections / result.totalConnections) * 100).toFixed(1);

      console.log(`${testNames[index]}:`);
      console.log(`   Success Rate: ${successRate}%`);
      console.log(`   Avg Latency: ${result.averageLatency.toFixed(1)}ms`);
      console.log(`   Errors: ${result.errors.length}`);
      console.log('');
    });

    return results;
  }
}

// CLI interface
async function main() {
  const serverUrl = process.env.WS_URL || 'ws://localhost:3001/ws';
  const authToken = process.env.AUTH_TOKEN;
  const testType = process.argv[2] || 'progressive';

  console.log('🧪 WebSocket Load Testing Tool');
  console.log(`Server: ${serverUrl}`);
  console.log(`Test Type: ${testType}`);
  console.log('');

  try {
    switch (testType.toLowerCase()) {
      case 'quick':
        await LoadTestScenarios.runQuickTest(serverUrl, authToken);
        break;
      case 'medium':
        await LoadTestScenarios.runMediumTest(serverUrl, authToken);
        break;
      case 'stress':
        await LoadTestScenarios.runStressTest(serverUrl, authToken);
        break;
      case 'progressive':
      default:
        await LoadTestScenarios.runProgressiveTest(serverUrl, authToken);
        break;
    }
  } catch (error) {
    console.error('❌ Load test failed:', error);
    process.exit(1);
  }
}

// Run if called directly
if (import.meta.url === `file://${process.argv[1]}`) {
  main();
}