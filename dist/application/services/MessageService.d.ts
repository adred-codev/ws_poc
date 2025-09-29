import { Message } from '../../domain/entities/Message';
import { ClientId } from '../../domain/value-objects/ClientId';
import { IMessageRepository } from '../../domain/repositories/IMessageRepository';
import { IClientRepository } from '../../domain/repositories/IClientRepository';
import { PublishPriceUpdateRequest, PublishPriceUpdateResponse } from '../../domain/use-cases/PublishPriceUpdateUseCase';
export interface MessageMetrics {
    messagesPublished: number;
    messagesDelivered: number;
    duplicatesDropped: number;
    averageLatency: number;
    peakLatency: number;
    errorRate: number;
}
export declare class MessageService {
    private readonly messageRepository;
    private readonly clientRepository;
    private readonly publishPriceUpdateUseCase;
    private metrics;
    private latencySum;
    private latencyCount;
    private errors;
    constructor(messageRepository: IMessageRepository, clientRepository: IClientRepository);
    publishPriceUpdate(request: PublishPriceUpdateRequest): Promise<PublishPriceUpdateResponse>;
    publishToClient(clientId: ClientId, message: Message): Promise<boolean>;
    broadcastToAllClients(message: Message): Promise<number>;
    sendHeartbeat(): Promise<number>;
    getMetrics(): MessageMetrics;
    resetMetrics(): void;
    private updateMetrics;
    private updateErrorRate;
}
//# sourceMappingURL=MessageService.d.ts.map