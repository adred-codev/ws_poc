import { EventEmitter } from 'events';
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
export declare class EnhancedMetricsService extends EventEmitter {
    private connections;
    private systemMetrics;
    private connectionStats;
    private messageStats;
    private startTime;
    private updateInterval;
    private isRunning;
    constructor();
    start(): Promise<void>;
    stop(): void;
    private updateSystemMetrics;
    private calculateRates;
    addConnection(id: string, remoteAddress: string): void;
    removeConnection(id: string): void;
    updateConnectionActivity(id: string, messageType: 'sent' | 'received', bytes?: number): void;
    getMetrics(): EnhancedMetricsData;
    getSimpleMetrics(): {
        connectionCount: number;
        memory: number;
        cpu: number;
    };
    getConnectionCount(): number;
    isHealthy(): boolean;
}
export declare const enhancedMetricsService: EnhancedMetricsService;
export {};
//# sourceMappingURL=enhanced-metrics.d.ts.map