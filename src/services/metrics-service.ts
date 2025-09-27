import { EventEmitter } from 'events';

export interface DetailedMetrics {
  connections: {
    active: number;
    total: number;
    successful: number;
    failed: number;
    rate: number;
    history: number[];
  };
  messages: {
    perSecond: number;
    total: number;
    delivered: number;
    failed: number;
    history: number[];
  };
  latency: {
    current: number;
    average: number;
    min: number;
    max: number;
    p95: number;
    p99: number;
    history: number[];
  };
  errors: {
    rate: number;
    total: number;
    types: Record<string, number>;
    recent: Array<{
      timestamp: number;
      type: string;
      message: string;
    }>;
  };
  system: {
    uptime: number;
    memory: {
      used: number;
      total: number;
      percentage: number;
    };
    cpu: number;
    health: 'healthy' | 'warning' | 'critical';
    components: {
      nats: 'connected' | 'disconnected' | 'error';
      websocket: 'healthy' | 'degraded' | 'critical';
      publisher: 'active' | 'idle' | 'error';
    };
  };
  loadTest: {
    isRunning: boolean;
    progress?: {
      phase: 'ramping' | 'sustained' | 'cleanup';
      connections: {
        attempted: number;
        successful: number;
        target: number;
      };
      duration: {
        elapsed: number;
        total: number;
      };
    };
  };
}

export interface MetricsSnapshot {
  timestamp: number;
  metrics: DetailedMetrics;
}

class MetricsService extends EventEmitter {
  private metrics: DetailedMetrics;
  private history: MetricsSnapshot[] = [];
  private startTime: number = Date.now();
  private intervals: Map<string, NodeJS.Timeout> = new Map();

  constructor() {
    super();
    this.metrics = this.initializeMetrics();
    this.startMetricsCollection();
  }

  private initializeMetrics(): DetailedMetrics {
    return {
      connections: {
        active: 0,
        total: 0,
        successful: 0,
        failed: 0,
        rate: 0,
        history: new Array(60).fill(0), // Last 60 seconds
      },
      messages: {
        perSecond: 0,
        total: 0,
        delivered: 0,
        failed: 0,
        history: new Array(60).fill(0),
      },
      latency: {
        current: 0,
        average: 0,
        min: Infinity,
        max: 0,
        p95: 0,
        p99: 0,
        history: new Array(60).fill(0),
      },
      errors: {
        rate: 0,
        total: 0,
        types: {},
        recent: [],
      },
      system: {
        uptime: 0,
        memory: {
          used: 0,
          total: 0,
          percentage: 0,
        },
        cpu: 0,
        health: 'healthy',
        components: {
          nats: 'disconnected',
          websocket: 'healthy',
          publisher: 'idle',
        },
      },
      loadTest: {
        isRunning: false,
      },
    };
  }

  private startMetricsCollection(): void {
    // Update metrics every second
    const metricsInterval = setInterval(() => {
      this.updateSystemMetrics();
      this.updateHistoryArrays();
      this.calculateRates();
      this.emit('metrics-updated', this.getMetrics());
    }, 1000);

    // Clean up old history every 5 minutes
    const cleanupInterval = setInterval(() => {
      this.cleanupHistory();
    }, 5 * 60 * 1000);

    this.intervals.set('metrics', metricsInterval);
    this.intervals.set('cleanup', cleanupInterval);
  }

  private updateSystemMetrics(): void {
    // Update uptime
    this.metrics.system.uptime = Date.now() - this.startTime;

    // Update memory usage
    const memUsage = process.memoryUsage();
    this.metrics.system.memory = {
      used: memUsage.heapUsed,
      total: memUsage.heapTotal,
      percentage: (memUsage.heapUsed / memUsage.heapTotal) * 100,
    };

    // Calculate health status
    this.updateHealthStatus();
  }

  private updateHealthStatus(): void {
    const { connections, errors, system } = this.metrics;

    // Determine overall health based on multiple factors
    let healthScore = 100;

    // Deduct points for high error rate
    if (errors.rate > 0.05) healthScore -= 30; // >5% error rate
    else if (errors.rate > 0.01) healthScore -= 15; // >1% error rate

    // Deduct points for high memory usage
    if (system.memory.percentage > 90) healthScore -= 20;
    else if (system.memory.percentage > 70) healthScore -= 10;

    // Deduct points for component issues
    if (system.components.nats !== 'connected') healthScore -= 25;
    if (system.components.websocket !== 'healthy') healthScore -= 25;
    if (system.components.publisher !== 'active') healthScore -= 10;

    // Set health status
    if (healthScore >= 80) {
      this.metrics.system.health = 'healthy';
    } else if (healthScore >= 60) {
      this.metrics.system.health = 'warning';
    } else {
      this.metrics.system.health = 'critical';
    }
  }

  private updateHistoryArrays(): void {
    // Shift arrays and add current values
    this.metrics.connections.history.shift();
    this.metrics.connections.history.push(this.metrics.connections.active);

    this.metrics.messages.history.shift();
    this.metrics.messages.history.push(this.metrics.messages.perSecond);

    this.metrics.latency.history.shift();
    this.metrics.latency.history.push(this.metrics.latency.current);
  }

  private calculateRates(): void {
    // Calculate connection rate (connections per minute)
    const connectionHistory = this.metrics.connections.history;
    const recentConnections = connectionHistory.slice(-10); // Last 10 seconds
    this.metrics.connections.rate = recentConnections.reduce((a, b) => a + b, 0) * 6; // * 6 to get per minute

    // Calculate error rate
    const totalOperations = this.metrics.messages.total + this.metrics.connections.total;
    this.metrics.errors.rate = totalOperations > 0 ? this.metrics.errors.total / totalOperations : 0;
  }

  private cleanupHistory(): void {
    // Keep only last 24 hours of snapshots
    const cutoff = Date.now() - 24 * 60 * 60 * 1000;
    this.history = this.history.filter(snapshot => snapshot.timestamp > cutoff);

    // Limit recent errors to last 100
    this.metrics.errors.recent = this.metrics.errors.recent.slice(-100);
  }

  // Public API methods
  public getMetrics(): DetailedMetrics {
    return JSON.parse(JSON.stringify(this.metrics)); // Deep clone
  }

  public getSnapshot(): MetricsSnapshot {
    return {
      timestamp: Date.now(),
      metrics: this.getMetrics(),
    };
  }

  public getHistory(hours: number = 1): MetricsSnapshot[] {
    const cutoff = Date.now() - hours * 60 * 60 * 1000;
    return this.history.filter(snapshot => snapshot.timestamp > cutoff);
  }

  // Connection tracking methods
  public recordConnection(success: boolean): void {
    this.metrics.connections.total++;
    if (success) {
      this.metrics.connections.successful++;
      this.metrics.connections.active++;
    } else {
      this.metrics.connections.failed++;
    }
  }

  public recordDisconnection(): void {
    this.metrics.connections.active = Math.max(0, this.metrics.connections.active - 1);
  }

  // Message tracking methods
  public recordMessage(success: boolean): void {
    this.metrics.messages.total++;
    if (success) {
      this.metrics.messages.delivered++;
    } else {
      this.metrics.messages.failed++;
    }
  }

  public updateMessageRate(rate: number): void {
    this.metrics.messages.perSecond = rate;
  }

  // Latency tracking methods
  public recordLatency(latency: number): void {
    this.metrics.latency.current = latency;
    this.metrics.latency.min = Math.min(this.metrics.latency.min, latency);
    this.metrics.latency.max = Math.max(this.metrics.latency.max, latency);

    // Calculate running average (simple moving average)
    this.metrics.latency.average = (this.metrics.latency.average * 0.9) + (latency * 0.1);
  }

  // Error tracking methods
  public recordError(type: string, message: string): void {
    this.metrics.errors.total++;
    this.metrics.errors.types[type] = (this.metrics.errors.types[type] || 0) + 1;
    this.metrics.errors.recent.push({
      timestamp: Date.now(),
      type,
      message,
    });
  }

  // Component status methods
  public setComponentStatus(component: keyof DetailedMetrics['system']['components'], status: string): void {
    (this.metrics.system.components[component] as any) = status;
  }

  // Load test tracking methods
  public startLoadTest(target: number): void {
    this.metrics.loadTest = {
      isRunning: true,
      progress: {
        phase: 'ramping',
        connections: {
          attempted: 0,
          successful: 0,
          target,
        },
        duration: {
          elapsed: 0,
          total: 0,
        },
      },
    };
  }

  public updateLoadTestProgress(attempted: number, successful: number, phase: 'ramping' | 'sustained' | 'cleanup'): void {
    if (this.metrics.loadTest.progress) {
      this.metrics.loadTest.progress.connections.attempted = attempted;
      this.metrics.loadTest.progress.connections.successful = successful;
      this.metrics.loadTest.progress.phase = phase;
    }
  }

  public endLoadTest(): void {
    this.metrics.loadTest.isRunning = false;
    delete this.metrics.loadTest.progress;
  }

  // Snapshot management
  public saveSnapshot(): void {
    const snapshot = this.getSnapshot();
    this.history.push(snapshot);
  }

  public shutdown(): void {
    // Clear all intervals
    for (const interval of this.intervals.values()) {
      clearInterval(interval);
    }
    this.intervals.clear();
    this.removeAllListeners();
  }
}

// Singleton instance
export const metricsService = new MetricsService();

// Save snapshots every 5 minutes
setInterval(() => {
  metricsService.saveSnapshot();
}, 5 * 60 * 1000);