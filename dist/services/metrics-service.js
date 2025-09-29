import { EventEmitter } from 'events';
class MetricsService extends EventEmitter {
    metrics;
    history = [];
    startTime = Date.now();
    intervals = new Map();
    constructor() {
        super();
        this.metrics = this.initializeMetrics();
        this.startMetricsCollection();
    }
    initializeMetrics() {
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
    startMetricsCollection() {
        // Track message counts for rate calculation
        let lastMessageCount = 0;
        let lastTimestamp = Date.now();
        // Update metrics every second
        const metricsInterval = setInterval(() => {
            this.updateSystemMetrics();
            this.calculateMessageRate(lastMessageCount, lastTimestamp);
            this.updateHistoryArrays();
            this.calculateRates();
            this.emit('metrics-updated', this.getMetrics());
            // Update tracking variables
            lastMessageCount = this.metrics.messages.total;
            lastTimestamp = Date.now();
        }, 1000);
        // Clean up old history every 5 minutes
        const cleanupInterval = setInterval(() => {
            this.cleanupHistory();
        }, 5 * 60 * 1000);
        this.intervals.set('metrics', metricsInterval);
        this.intervals.set('cleanup', cleanupInterval);
    }
    updateSystemMetrics() {
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
    updateHealthStatus() {
        const { connections, errors, system } = this.metrics;
        // Determine overall health based on multiple factors
        let healthScore = 100;
        // Deduct points for high error rate
        if (errors.rate > 0.05)
            healthScore -= 30; // >5% error rate
        else if (errors.rate > 0.01)
            healthScore -= 15; // >1% error rate
        // Deduct points for high memory usage
        if (system.memory.percentage > 90)
            healthScore -= 20;
        else if (system.memory.percentage > 70)
            healthScore -= 10;
        // Deduct points for component issues
        if (system.components.nats !== 'connected')
            healthScore -= 25;
        if (system.components.websocket !== 'healthy')
            healthScore -= 25;
        if (system.components.publisher !== 'active')
            healthScore -= 10;
        // Set health status
        if (healthScore >= 80) {
            this.metrics.system.health = 'healthy';
        }
        else if (healthScore >= 60) {
            this.metrics.system.health = 'warning';
        }
        else {
            this.metrics.system.health = 'critical';
        }
    }
    updateHistoryArrays() {
        // Shift arrays and add current values
        this.metrics.connections.history.shift();
        this.metrics.connections.history.push(this.metrics.connections.active);
        this.metrics.messages.history.shift();
        this.metrics.messages.history.push(this.metrics.messages.perSecond);
        this.metrics.latency.history.shift();
        this.metrics.latency.history.push(this.metrics.latency.current);
    }
    calculateRates() {
        // Calculate connection rate (connections per minute)
        const connectionHistory = this.metrics.connections.history;
        const recentConnections = connectionHistory.slice(-10); // Last 10 seconds
        this.metrics.connections.rate = recentConnections.reduce((a, b) => a + b, 0) * 6; // * 6 to get per minute
        // Calculate error rate
        const totalOperations = this.metrics.messages.total + this.metrics.connections.total;
        this.metrics.errors.rate = totalOperations > 0 ? this.metrics.errors.total / totalOperations : 0;
    }
    cleanupHistory() {
        // Keep only last 24 hours of snapshots
        const cutoff = Date.now() - 24 * 60 * 60 * 1000;
        this.history = this.history.filter(snapshot => snapshot.timestamp > cutoff);
        // Limit recent errors to last 100
        this.metrics.errors.recent = this.metrics.errors.recent.slice(-100);
    }
    // Public API methods
    getMetrics() {
        return JSON.parse(JSON.stringify(this.metrics)); // Deep clone
    }
    getSnapshot() {
        return {
            timestamp: Date.now(),
            metrics: this.getMetrics(),
        };
    }
    getHistory(hours = 1) {
        const cutoff = Date.now() - hours * 60 * 60 * 1000;
        return this.history.filter(snapshot => snapshot.timestamp > cutoff);
    }
    // Connection tracking methods
    recordConnection(success) {
        this.metrics.connections.total++;
        if (success) {
            this.metrics.connections.successful++;
            this.metrics.connections.active++;
        }
        else {
            this.metrics.connections.failed++;
        }
    }
    recordDisconnection() {
        this.metrics.connections.active = Math.max(0, this.metrics.connections.active - 1);
    }
    // Message tracking methods
    recordMessage(success) {
        this.metrics.messages.total++;
        if (success) {
            this.metrics.messages.delivered++;
        }
        else {
            this.metrics.messages.failed++;
        }
    }
    updateMessageRate(rate) {
        this.metrics.messages.perSecond = rate;
    }
    calculateMessageRate(lastMessageCount, lastTimestamp) {
        const currentTime = Date.now();
        const timeDelta = (currentTime - lastTimestamp) / 1000; // Convert to seconds
        const messageDelta = this.metrics.messages.total - lastMessageCount;
        if (timeDelta > 0) {
            this.metrics.messages.perSecond = messageDelta / timeDelta;
        }
    }
    // Latency tracking methods
    recordLatency(latency) {
        this.metrics.latency.current = latency;
        this.metrics.latency.min = Math.min(this.metrics.latency.min, latency);
        this.metrics.latency.max = Math.max(this.metrics.latency.max, latency);
        // Calculate running average (simple moving average)
        this.metrics.latency.average = (this.metrics.latency.average * 0.9) + (latency * 0.1);
    }
    // Error tracking methods
    recordError(type, message) {
        this.metrics.errors.total++;
        this.metrics.errors.types[type] = (this.metrics.errors.types[type] || 0) + 1;
        this.metrics.errors.recent.push({
            timestamp: Date.now(),
            type,
            message,
        });
    }
    // Component status methods
    setComponentStatus(component, status) {
        this.metrics.system.components[component] = status;
    }
    // Load test tracking methods
    startLoadTest(target) {
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
    updateLoadTestProgress(attempted, successful, phase) {
        if (this.metrics.loadTest.progress) {
            this.metrics.loadTest.progress.connections.attempted = attempted;
            this.metrics.loadTest.progress.connections.successful = successful;
            this.metrics.loadTest.progress.phase = phase;
        }
    }
    endLoadTest() {
        this.metrics.loadTest.isRunning = false;
        delete this.metrics.loadTest.progress;
    }
    // Snapshot management
    saveSnapshot() {
        const snapshot = this.getSnapshot();
        this.history.push(snapshot);
    }
    shutdown() {
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
//# sourceMappingURL=metrics-service.js.map