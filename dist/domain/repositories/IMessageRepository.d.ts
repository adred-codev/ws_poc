import { Message } from '../entities/Message';
import { ClientId } from '../value-objects/ClientId';
export interface IMessageRepository {
    publishToClient(clientId: ClientId, message: Message): Promise<boolean>;
    broadcastToAllClients(message: Message): Promise<number>;
    broadcastToClients(clientIds: ClientId[], message: Message): Promise<number>;
}
//# sourceMappingURL=IMessageRepository.d.ts.map