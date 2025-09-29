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
export declare class LoadTestScenarios {
    static runQuickTest(serverUrl: string, authToken?: string): Promise<TestMetrics>;
    static runMediumTest(serverUrl: string, authToken?: string): Promise<TestMetrics>;
    static runStressTest(serverUrl: string, authToken?: string): Promise<TestMetrics>;
    static runProgressiveTest(serverUrl: string, authToken?: string): Promise<TestMetrics[]>;
}
export {};
//# sourceMappingURL=load-test.d.ts.map