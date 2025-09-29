import { ClientId } from '../value-objects/ClientId';
import { IClientRepository } from '../repositories/IClientRepository';
import { IMessageRepository } from '../repositories/IMessageRepository';
export interface ConnectClientRequest {
    ip: string;
    userAgent: string;
    token?: string;
}
export interface ConnectClientResponse {
    clientId: ClientId;
    success: boolean;
    message: string;
}
export declare class ConnectClientUseCase {
    private readonly clientRepository;
    private readonly messageRepository;
    constructor(clientRepository: IClientRepository, messageRepository: IMessageRepository);
    execute(request: ConnectClientRequest): Promise<ConnectClientResponse>;
    private sendWelcomeMessage;
}
//# sourceMappingURL=ConnectClientUseCase.d.ts.map