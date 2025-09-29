import { HeartbeatMessage } from '../../domain/entities/Message';
import { PublishPriceUpdateUseCase } from '../../domain/use-cases/PublishPriceUpdateUseCase';
export class MessageService {
    messageRepository;
    clientRepository;
    publishPriceUpdateUseCase;
    metrics = {
        messagesPublished: 0,
        messagesDelivered: 0,
        duplicatesDropped: 0,
        averageLatency: 0,
        peakLatency: 0,
        errorRate: 0
    };
    latencySum = 0;
    latencyCount = 0;
    errors = 0;
    constructor(messageRepository, clientRepository) {
        this.messageRepository = messageRepository;
        this.clientRepository = clientRepository;
        this.publishPriceUpdateUseCase = new PublishPriceUpdateUseCase(messageRepository, clientRepository);
    }
    async publishPriceUpdate(request) {
        const startTime = Date.now();
        try {
            const result = await this.publishPriceUpdateUseCase.execute(request);
            this.updateMetrics(startTime, result.messagesSent, result.duplicatesDropped);
            return result;
        }
        catch (error) {
            this.errors++;
            this.updateErrorRate();
            throw error;
        }
    }
    async publishToClient(clientId, message) {
        const startTime = Date.now();
        try {
            const success = await this.messageRepository.publishToClient(clientId, message);
            if (success) {
                this.updateMetrics(startTime, 1, 0);
            }
            return success;
        }
        catch (error) {
            this.errors++;
            this.updateErrorRate();
            throw error;
        }
    }
    async broadcastToAllClients(message) {
        const startTime = Date.now();
        try {
            const messagesSent = await this.messageRepository.broadcastToAllClients(message);
            this.updateMetrics(startTime, messagesSent, 0);
            return messagesSent;
        }
        catch (error) {
            this.errors++;
            this.updateErrorRate();
            throw error;
        }
    }
    async sendHeartbeat() {
        const clientCount = await this.clientRepository.count();
        const heartbeat = new HeartbeatMessage(clientCount);
        return await this.broadcastToAllClients(heartbeat);
    }
    getMetrics() {
        return { ...this.metrics };
    }
    resetMetrics() {
        this.metrics = {
            messagesPublished: 0,
            messagesDelivered: 0,
            duplicatesDropped: 0,
            averageLatency: 0,
            peakLatency: 0,
            errorRate: 0
        };
        this.latencySum = 0;
        this.latencyCount = 0;
        this.errors = 0;
    }
    updateMetrics(startTime, messagesSent, duplicatesDropped) {
        const latency = Date.now() - startTime;
        this.metrics.messagesPublished++;
        this.metrics.messagesDelivered += messagesSent;
        this.metrics.duplicatesDropped += duplicatesDropped;
        // Update latency metrics
        this.latencySum += latency;
        this.latencyCount++;
        this.metrics.averageLatency = this.latencySum / this.latencyCount;
        if (latency > this.metrics.peakLatency) {
            this.metrics.peakLatency = latency;
        }
        this.updateErrorRate();
    }
    updateErrorRate() {
        const totalOperations = this.metrics.messagesPublished + this.errors;
        this.metrics.errorRate = totalOperations > 0 ? this.errors / totalOperations : 0;
    }
}
//# sourceMappingURL=MessageService.js.map