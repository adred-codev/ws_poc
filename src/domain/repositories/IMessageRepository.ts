import { Message } from '../entities/Message.js';
import { ClientId } from '../value-objects/ClientId.js';

export interface IMessageRepository {
  publishToClient(clientId: ClientId, message: Message): Promise<boolean>;
  broadcastToAllClients(message: Message): Promise<number>;
  broadcastToClients(clientIds: ClientId[], message: Message): Promise<number>;
}