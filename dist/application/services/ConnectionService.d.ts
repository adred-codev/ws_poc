import { Client } from '../../domain/entities/Client';
import { ClientId } from '../../domain/value-objects/ClientId';
import { IClientRepository } from '../../domain/repositories/IClientRepository';
import { ConnectClientRequest, ConnectClientResponse } from '../../domain/use-cases/ConnectClientUseCase';
import { IMessageRepository } from '../../domain/repositories/IMessageRepository';
export interface ConnectionMetrics {
    totalConnections: number;
    currentConnections: number;
    averageConnectionDuration: number;
    activeClients: number;
}
export declare class ConnectionService {
    private readonly clientRepository;
    private readonly messageRepository;
    private readonly connectClientUseCase;
    constructor(clientRepository: IClientRepository, messageRepository: IMessageRepository);
    connectClient(request: ConnectClientRequest): Promise<ConnectClientResponse>;
    disconnectClient(clientId: ClientId): Promise<void>;
    findClient(clientId: ClientId): Promise<Client | null>;
    getConnectionMetrics(): Promise<ConnectionMetrics>;
    updateClientActivity(clientId: ClientId): Promise<void>;
    cleanupInactiveClients(timeoutMs?: number): Promise<number>;
    getAllClients(): Promise<Client[]>;
    getActiveClients(timeoutMs?: number): Promise<Client[]>;
}
//# sourceMappingURL=ConnectionService.d.ts.map