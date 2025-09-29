export class HealthController {
    connectionService;
    messageService;
    natsSubscriber;
    startTime = Date.now();
    constructor(connectionService, messageService, natsSubscriber) {
        this.connectionService = connectionService;
        this.messageService = messageService;
        this.natsSubscriber = natsSubscriber;
    }
    async getHealth(req, res) {
        try {
            const uptime = Date.now() - this.startTime;
            const connectionMetrics = await this.connectionService.getConnectionMetrics();
            const healthData = {
                status: 'healthy',
                uptime: Math.floor(uptime / 1000),
                nats: this.natsSubscriber.isConnected() ? 'connected' : 'disconnected',
                websocket: {
                    currentConnections: connectionMetrics.currentConnections,
                    totalConnections: connectionMetrics.totalConnections,
                    activeClients: connectionMetrics.activeClients
                },
                timestamp: Date.now()
            };
            res.json(healthData);
        }
        catch (error) {
            console.error('❌ Error getting health status:', error);
            res.status(500).json({
                status: 'unhealthy',
                error: 'Internal server error',
                timestamp: Date.now()
            });
        }
    }
    async getStats(req, res) {
        try {
            const uptime = Date.now() - this.startTime;
            const connectionMetrics = await this.connectionService.getConnectionMetrics();
            const messageMetrics = this.messageService.getMetrics();
            const statsData = {
                uptime: Math.floor(uptime / 1000),
                connections: connectionMetrics,
                messages: messageMetrics,
                performance: {
                    averageLatency: `${messageMetrics.averageLatency.toFixed(2)}ms`,
                    peakLatency: `${messageMetrics.peakLatency}ms`,
                    errorRate: `${(messageMetrics.errorRate * 100).toFixed(2)}%`
                },
                timestamp: Date.now()
            };
            res.json(statsData);
        }
        catch (error) {
            console.error('❌ Error getting stats:', error);
            res.status(500).json({
                error: 'Internal server error',
                timestamp: Date.now()
            });
        }
    }
    async getMetrics(req, res) {
        try {
            const connectionMetrics = await this.connectionService.getConnectionMetrics();
            const messageMetrics = this.messageService.getMetrics();
            const uptime = Date.now() - this.startTime;
            // Calculate additional metrics
            const apiRequestsSaved = messageMetrics.messagesDelivered * 0.9; // Assuming 90% reduction
            const bandwidthSaved = (messageMetrics.messagesDelivered * 5000) / (1024 * 1024); // ~5KB per message saved
            const metricsData = {
                connections: connectionMetrics.currentConnections,
                messagesDelivered: messageMetrics.messagesDelivered,
                duplicatesDropped: messageMetrics.duplicatesDropped,
                averageLatency: messageMetrics.averageLatency,
                peakLatency: messageMetrics.peakLatency,
                apiRequestsSaved: Math.floor(apiRequestsSaved),
                bandwidthSaved: bandwidthSaved.toFixed(2),
                errorRate: messageMetrics.errorRate,
                uptime: Math.floor(uptime / 1000),
                timestamp: Date.now()
            };
            res.json(metricsData);
        }
        catch (error) {
            console.error('❌ Error getting metrics:', error);
            res.status(500).json({
                error: 'Internal server error',
                timestamp: Date.now()
            });
        }
    }
    async resetMetrics(req, res) {
        try {
            this.messageService.resetMetrics();
            res.json({
                message: 'Metrics reset successfully',
                timestamp: Date.now()
            });
        }
        catch (error) {
            console.error('❌ Error resetting metrics:', error);
            res.status(500).json({
                error: 'Internal server error',
                timestamp: Date.now()
            });
        }
    }
}
//# sourceMappingURL=HealthController.js.map