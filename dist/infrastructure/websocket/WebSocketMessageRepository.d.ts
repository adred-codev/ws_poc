import { WebSocket } from 'ws';
import { Message } from '../../domain/entities/Message';
import { ClientId } from '../../domain/value-objects/ClientId';
import { IMessageRepository } from '../../domain/repositories/IMessageRepository';
import { IClientRepository } from '../../domain/repositories/IClientRepository';
export interface WebSocketConnection {
    clientId: ClientId;
    socket: WebSocket;
}
export declare class WebSocketMessageRepository implements IMessageRepository {
    private readonly clientRepository;
    private readonly connections;
    constructor(clientRepository: IClientRepository);
    addConnection(clientId: ClientId, socket: WebSocket): void;
    removeConnection(clientId: ClientId): void;
    publishToClient(clientId: ClientId, message: Message): Promise<boolean>;
    broadcastToAllClients(message: Message): Promise<number>;
    broadcastToClients(clientIds: ClientId[], message: Message): Promise<number>;
    getActiveConnections(): WebSocketConnection[];
    cleanupDeadConnections(): number;
    getConnectionCount(): number;
}
//# sourceMappingURL=WebSocketMessageRepository.d.ts.map