import { UpdateFrequencies, NatsSubjects, OdinConfig } from '../types/odin.types';
export declare const updateFrequencies: UpdateFrequencies;
export declare const subjects: NatsSubjects;
export declare const metricsConfig: MetricsConfig;
export declare const scalingConfig: ScalingConfig;
export declare const migrationConfig: MigrationConfig;
export declare const config: OdinConfig;
export interface MetricsConfig {
    metrics: string[];
    reportInterval: number;
    thresholds: {
        latency: number;
        errorRate: number;
        connections: number;
    };
}
export interface ScalingConfig {
    maxConnectionsPerInstance: number;
    autoScaleThreshold: number;
    cooldownPeriod: number;
    minInstances: number;
    maxInstances: number;
}
export interface MigrationConfig {
    enabled: boolean;
    pollingEndpoint: string;
    websocketEndpoint: string;
    rolloutPercentage: number;
    fallbackEnabled: boolean;
    dualModeDuration: number;
}
//# sourceMappingURL=odin.config.d.ts.map