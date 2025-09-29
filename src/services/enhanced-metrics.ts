import * as si from 'systeminformation';
import pidusage from 'pidusage';
import { EventEmitter } from 'events';
import { cpus, loadavg, totalmem, freemem } from 'os';

interface SystemMetrics {
  cpu: {
    percent: number;
    cores: number;
    loadAvg: number[];
  };
  memory: {
    total: number;
    used: number;
    free: number;
    percent: number;
  };
  process: {
    cpu: number;
    memory: number;
    pid: number;
    uptime: number;
  };
  timestamp: number;
}

interface ConnectionMetrics {
  active: number;
  total: number;
  peak: number;
  connectionsPerSecond: number;
  averageDuration: number;
  connections: ConnectionInfo[];
}

interface ConnectionInfo {
  id: string;
  connectedAt: number;
  remoteAddress: string;
  messagesSent: number;
  messagesReceived: number;
  bytesSent: number;
  bytesReceived: number;
  lastActivity: number;
}

interface EnhancedMetricsData {
  system: SystemMetrics;
  connections: ConnectionMetrics;
  performance: {
    memoryMB: number;
    cpuPercent: number;
    activeConnections: number;
    messagesPerSecond: number;
  };
  uptime: number;
  timestamp: number;
}

export class EnhancedMetricsService extends EventEmitter {
  private connections = new Map<string, ConnectionInfo>();
  private systemMetrics: SystemMetrics | null = null;
  private connectionStats = {
    total: 0,
    peak: 0,
    lastSecondConnections: 0,
    connectionsPerSecond: 0,
    lastConnectionTime: Date.now()
  };
  private messageStats = {
    lastSecondMessages: 0,
    messagesPerSecond: 0,
    lastMessageTime: Date.now()
  };
  private startTime = Date.now();
  private updateInterval: NodeJS.Timeout | null = null;
  private isRunning = false;

  constructor() {
    super();
  }

  async start(): Promise<void> {
    if (this.isRunning) return;

    this.isRunning = true;
    console.log('🔍 Starting enhanced metrics collection...');

    // Initial system info
    await this.updateSystemMetrics();

    // Update metrics every 5 seconds
    this.updateInterval = setInterval(async () => {
      try {
        await this.updateSystemMetrics();
        this.calculateRates();
        this.emit('metrics-updated', this.getMetrics());
      } catch (error) {
        console.error('❌ Error updating enhanced metrics:', error);
      }
    }, 5000);

    console.log('✅ Enhanced metrics collection started');
  }

  stop(): void {
    if (!this.isRunning) return;

    this.isRunning = false;
    if (this.updateInterval) {
      clearInterval(this.updateInterval);
      this.updateInterval = null;
    }
    console.log('⏹️ Enhanced metrics collection stopped');
  }

  private async updateSystemMetrics(): Promise<void> {
    try {
      // Get system metrics using systeminformation
      const [cpuInfo, memInfo, loadInfo, processStats] = await Promise.all([
        si.currentLoad(),
        si.mem(),
        si.osInfo(),
        pidusage(process.pid).catch(() => ({
          cpu: 0,
          memory: process.memoryUsage().rss,
          pid: process.pid
        }))
      ]);

      this.systemMetrics = {
        cpu: {
          percent: Number(cpuInfo.currentLoad.toFixed(2)),
          cores: cpuInfo.cpus?.length || cpus().length,
          loadAvg: loadavg()
        },
        memory: {
          total: Math.round(memInfo.total / 1024 / 1024), // MB
          used: Math.round(memInfo.used / 1024 / 1024), // MB
          free: Math.round(memInfo.free / 1024 / 1024), // MB
          percent: Number(((memInfo.used / memInfo.total) * 100).toFixed(2))
        },
        process: {
          cpu: Number(processStats.cpu.toFixed(2)),
          memory: Math.round(processStats.memory / 1024 / 1024), // MB
          pid: processStats.pid,
          uptime: Math.floor(process.uptime())
        },
        timestamp: Date.now()
      };

    } catch (error) {
      console.error('Error collecting system metrics:', error);

      // Fallback to basic Node.js metrics
      const memUsage = process.memoryUsage();
      this.systemMetrics = {
        cpu: {
          percent: 0,
          cores: cpus().length,
          loadAvg: loadavg()
        },
        memory: {
          total: Math.round(totalmem() / 1024 / 1024),
          used: Math.round((totalmem() - freemem()) / 1024 / 1024),
          free: Math.round(freemem() / 1024 / 1024),
          percent: 0
        },
        process: {
          cpu: 0,
          memory: Math.round(memUsage.rss / 1024 / 1024),
          pid: process.pid,
          uptime: Math.floor(process.uptime())
        },
        timestamp: Date.now()
      };
    }
  }

  private calculateRates(): void {
    const now = Date.now();

    // Calculate connections per second
    const connectionTimeDelta = (now - this.connectionStats.lastConnectionTime) / 1000;
    if (connectionTimeDelta >= 1) {
      this.connectionStats.connectionsPerSecond = this.connectionStats.lastSecondConnections / connectionTimeDelta;
      this.connectionStats.lastSecondConnections = 0;
      this.connectionStats.lastConnectionTime = now;
    }

    // Calculate messages per second
    const messageTimeDelta = (now - this.messageStats.lastMessageTime) / 1000;
    if (messageTimeDelta >= 1) {
      this.messageStats.messagesPerSecond = this.messageStats.lastSecondMessages / messageTimeDelta;
      this.messageStats.lastSecondMessages = 0;
      this.messageStats.lastMessageTime = now;
    }
  }

  // Connection tracking methods
  addConnection(id: string, remoteAddress: string): void {
    const connectionInfo: ConnectionInfo = {
      id,
      connectedAt: Date.now(),
      remoteAddress,
      messagesSent: 0,
      messagesReceived: 0,
      bytesSent: 0,
      bytesReceived: 0,
      lastActivity: Date.now()
    };

    this.connections.set(id, connectionInfo);
    this.connectionStats.total++;
    this.connectionStats.lastSecondConnections++;

    // Update peak connections
    const currentActive = this.connections.size;
    if (currentActive > this.connectionStats.peak) {
      this.connectionStats.peak = currentActive;
    }

    console.log(`📱 Connection added: ${id} (${this.connections.size} active)`);
  }

  removeConnection(id: string): void {
    const connection = this.connections.get(id);
    if (connection) {
      this.connections.delete(id);
      const duration = Date.now() - connection.connectedAt;
      console.log(`📱 Connection removed: ${id} (${this.connections.size} active, duration: ${Math.round(duration/1000)}s)`);
    }
  }

  updateConnectionActivity(id: string, messageType: 'sent' | 'received', bytes: number = 0): void {
    const connection = this.connections.get(id);
    if (connection) {
      connection.lastActivity = Date.now();
      this.messageStats.lastSecondMessages++;

      if (messageType === 'sent') {
        connection.messagesSent++;
        connection.bytesSent += bytes;
      } else {
        connection.messagesReceived++;
        connection.bytesReceived += bytes;
      }
    }
  }

  // Get comprehensive metrics
  getMetrics(): EnhancedMetricsData {
    if (!this.systemMetrics) {
      throw new Error('System metrics not initialized. Call start() first.');
    }

    const now = Date.now();
    const uptime = now - this.startTime;

    // Calculate average connection duration
    const activeConnections = Array.from(this.connections.values());
    const averageDuration = activeConnections.length > 0
      ? activeConnections.reduce((sum, conn) => sum + (now - conn.connectedAt), 0) / activeConnections.length / 1000
      : 0;

    const connectionMetrics: ConnectionMetrics = {
      active: this.connections.size,
      total: this.connectionStats.total,
      peak: this.connectionStats.peak,
      connectionsPerSecond: this.connectionStats.connectionsPerSecond,
      averageDuration: Number(averageDuration.toFixed(2)),
      connections: activeConnections.map(conn => ({
        ...conn,
        duration: Math.round((now - conn.connectedAt) / 1000),
        idleTime: Math.round((now - conn.lastActivity) / 1000)
      })) as any
    };

    const performance = {
      memoryMB: this.systemMetrics.process.memory,
      cpuPercent: this.systemMetrics.cpu.percent,
      activeConnections: this.connections.size,
      messagesPerSecond: this.messageStats.messagesPerSecond
    };

    return {
      system: this.systemMetrics,
      connections: connectionMetrics,
      performance,
      uptime: Math.round(uptime / 1000),
      timestamp: now
    };
  }

  // Get simple metrics for React client compatibility
  getSimpleMetrics() {
    if (!this.systemMetrics) {
      return {
        connectionCount: 0,
        memory: 0,
        cpu: 0
      };
    }

    return {
      connectionCount: this.connections.size,
      memory: this.systemMetrics.process.memory,
      cpu: this.systemMetrics.cpu.percent
    };
  }

  // Get connection count for external use
  getConnectionCount(): number {
    return this.connections.size;
  }

  // Health check
  isHealthy(): boolean {
    return this.isRunning && this.systemMetrics !== null;
  }
}

// Export singleton instance
export const enhancedMetricsService = new EnhancedMetricsService();