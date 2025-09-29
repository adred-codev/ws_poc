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
declare class MetricsService extends EventEmitter {
    private metrics;
    private history;
    private startTime;
    private intervals;
    constructor();
    private initializeMetrics;
    private startMetricsCollection;
    private updateSystemMetrics;
    private updateHealthStatus;
    private updateHistoryArrays;
    private calculateRates;
    private cleanupHistory;
    getMetrics(): DetailedMetrics;
    getSnapshot(): MetricsSnapshot;
    getHistory(hours?: number): MetricsSnapshot[];
    recordConnection(success: boolean): void;
    recordDisconnection(): void;
    recordMessage(success: boolean): void;
    updateMessageRate(rate: number): void;
    private calculateMessageRate;
    recordLatency(latency: number): void;
    recordError(type: string, message: string): void;
    setComponentStatus(component: keyof DetailedMetrics['system']['components'], status: string): void;
    startLoadTest(target: number): void;
    updateLoadTestProgress(attempted: number, successful: number, phase: 'ramping' | 'sustained' | 'cleanup'): void;
    endLoadTest(): void;
    saveSnapshot(): void;
    shutdown(): void;
}
export declare const metricsService: MetricsService;
export {};
//# sourceMappingURL=metrics-service.d.ts.map