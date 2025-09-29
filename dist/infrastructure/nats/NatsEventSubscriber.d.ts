import { MessageService } from '../../application/services/MessageService';
export interface NatsConfig {
    url: string;
    reconnect?: boolean;
    maxReconnectAttempts?: number;
    reconnectTimeWait?: number;
}
export declare class NatsEventSubscriber {
    private readonly config;
    private readonly messageService;
    private nats;
    private subscriptions;
    constructor(config: NatsConfig, messageService: MessageService);
    connect(): Promise<void>;
    subscribeToOdinEvents(): Promise<void>;
    private subscribeToSubject;
    private handlePriceUpdate;
    private handleBatchUpdate;
    private handleTradeEvent;
    disconnect(): Promise<void>;
    isConnected(): boolean;
}
//# sourceMappingURL=NatsEventSubscriber.d.ts.map