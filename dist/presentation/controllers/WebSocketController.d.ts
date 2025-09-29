import { WebSocket } from 'ws';
import { IncomingMessage } from 'http';
import { ConnectionService } from '../../application/services/ConnectionService';
import { MessageService } from '../../application/services/MessageService';
import { WebSocketMessageRepository } from '../../infrastructure/websocket/WebSocketMessageRepository';
export interface ClientMessage {
    type: string;
    timestamp?: number;
    [key: string]: any;
}
export declare class WebSocketController {
    private readonly connectionService;
    private readonly messageService;
    private readonly webSocketRepository;
    constructor(connectionService: ConnectionService, messageService: MessageService, webSocketRepository: WebSocketMessageRepository);
    handleConnection(socket: WebSocket, request: IncomingMessage): Promise<void>;
    private setupMessageHandlers;
    private setupDisconnectHandler;
    private handleClientMessage;
    private handlePing;
    private handleSubscription;
    private extractTokenFromRequest;
    private startHeartbeat;
}
//# sourceMappingURL=WebSocketController.d.ts.map