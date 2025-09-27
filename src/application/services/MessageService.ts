import { Message, PriceUpdateMessage, TradeExecutedMessage, HeartbeatMessage, MessageSource } from '../../domain/entities/Message';
import { ClientId } from '../../domain/value-objects/ClientId';
import { IMessageRepository } from '../../domain/repositories/IMessageRepository';
import { IClientRepository } from '../../domain/repositories/IClientRepository';
import { PublishPriceUpdateUseCase, PublishPriceUpdateRequest, PublishPriceUpdateResponse } from '../../domain/use-cases/PublishPriceUpdateUseCase';

export interface MessageMetrics {
  messagesPublished: number;
  messagesDelivered: number;
  duplicatesDropped: number;
  averageLatency: number;
  peakLatency: number;
  errorRate: number;
}

export class MessageService {
  private readonly publishPriceUpdateUseCase: PublishPriceUpdateUseCase;
  private metrics: MessageMetrics = {
    messagesPublished: 0,
    messagesDelivered: 0,
    duplicatesDropped: 0,
    averageLatency: 0,
    peakLatency: 0,
    errorRate: 0
  };
  private latencySum = 0;
  private latencyCount = 0;
  private errors = 0;

  constructor(
    private readonly messageRepository: IMessageRepository,
    private readonly clientRepository: IClientRepository
  ) {
    this.publishPriceUpdateUseCase = new PublishPriceUpdateUseCase(
      messageRepository,
      clientRepository
    );
  }

  async publishPriceUpdate(request: PublishPriceUpdateRequest): Promise<PublishPriceUpdateResponse> {
    const startTime = Date.now();

    try {
      const result = await this.publishPriceUpdateUseCase.execute(request);

      this.updateMetrics(startTime, result.messagesSent, result.duplicatesDropped);

      return result;
    } catch (error) {
      this.errors++;
      this.updateErrorRate();
      throw error;
    }
  }

  async publishToClient(clientId: ClientId, message: Message): Promise<boolean> {
    const startTime = Date.now();

    try {
      const success = await this.messageRepository.publishToClient(clientId, message);

      if (success) {
        this.updateMetrics(startTime, 1, 0);
      }

      return success;
    } catch (error) {
      this.errors++;
      this.updateErrorRate();
      throw error;
    }
  }

  async broadcastToAllClients(message: Message): Promise<number> {
    const startTime = Date.now();

    try {
      const messagesSent = await this.messageRepository.broadcastToAllClients(message);

      this.updateMetrics(startTime, messagesSent, 0);

      return messagesSent;
    } catch (error) {
      this.errors++;
      this.updateErrorRate();
      throw error;
    }
  }

  async sendHeartbeat(): Promise<number> {
    const clientCount = await this.clientRepository.count();
    const heartbeat = new HeartbeatMessage(clientCount);

    return await this.broadcastToAllClients(heartbeat);
  }

  getMetrics(): MessageMetrics {
    return { ...this.metrics };
  }

  resetMetrics(): void {
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

  private updateMetrics(startTime: number, messagesSent: number, duplicatesDropped: number): void {
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

  private updateErrorRate(): void {
    const totalOperations = this.metrics.messagesPublished + this.errors;
    this.metrics.errorRate = totalOperations > 0 ? this.errors / totalOperations : 0;
  }
}