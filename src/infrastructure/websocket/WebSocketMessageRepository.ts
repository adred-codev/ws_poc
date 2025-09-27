import { WebSocket } from 'ws';
import { Message } from '../../domain/entities/Message';
import { ClientId } from '../../domain/value-objects/ClientId';
import { IMessageRepository } from '../../domain/repositories/IMessageRepository';
import { IClientRepository } from '../../domain/repositories/IClientRepository';

export interface WebSocketConnection {
  clientId: ClientId;
  socket: WebSocket;
}

export class WebSocketMessageRepository implements IMessageRepository {
  private readonly connections: Map<string, WebSocket> = new Map();

  constructor(private readonly clientRepository: IClientRepository) {}

  addConnection(clientId: ClientId, socket: WebSocket): void {
    this.connections.set(clientId.value, socket);
  }

  removeConnection(clientId: ClientId): void {
    this.connections.delete(clientId.value);
  }

  async publishToClient(clientId: ClientId, message: Message): Promise<boolean> {
    const socket = this.connections.get(clientId.value);

    if (!socket || socket.readyState !== WebSocket.OPEN) {
      return false;
    }

    try {
      const messageData = JSON.stringify(message.toJSON());
      socket.send(messageData);

      // Update client activity
      const client = await this.clientRepository.findById(clientId);
      if (client) {
        client.incrementMessageCount();
        await this.clientRepository.save(client);
      }

      return true;
    } catch (error) {
      console.error(`Failed to send message to client ${clientId.value}:`, error);
      return false;
    }
  }

  async broadcastToAllClients(message: Message): Promise<number> {
    const allClientIds = Array.from(this.connections.keys()).map(id => new ClientId(id));
    return await this.broadcastToClients(allClientIds, message);
  }

  async broadcastToClients(clientIds: ClientId[], message: Message): Promise<number> {
    let sentCount = 0;

    for (const clientId of clientIds) {
      const success = await this.publishToClient(clientId, message);
      if (success) {
        sentCount++;
      }
    }

    return sentCount;
  }

  getActiveConnections(): WebSocketConnection[] {
    const connections: WebSocketConnection[] = [];

    for (const [clientIdValue, socket] of this.connections) {
      if (socket.readyState === WebSocket.OPEN) {
        connections.push({
          clientId: new ClientId(clientIdValue),
          socket
        });
      }
    }

    return connections;
  }

  cleanupDeadConnections(): number {
    let cleaned = 0;

    for (const [clientIdValue, socket] of this.connections) {
      if (socket.readyState !== WebSocket.OPEN) {
        this.connections.delete(clientIdValue);
        cleaned++;
      }
    }

    return cleaned;
  }

  getConnectionCount(): number {
    return this.connections.size;
  }
}