import { Client } from '../../domain/entities/Client';
import { ClientId } from '../../domain/value-objects/ClientId';
import { IClientRepository } from '../../domain/repositories/IClientRepository';

export class InMemoryClientRepository implements IClientRepository {
  private readonly clients: Map<string, Client> = new Map();

  async save(client: Client): Promise<void> {
    this.clients.set(client.id.value, client);
  }

  async findById(id: ClientId): Promise<Client | null> {
    return this.clients.get(id.value) || null;
  }

  async findAll(): Promise<Client[]> {
    return Array.from(this.clients.values());
  }

  async remove(id: ClientId): Promise<void> {
    this.clients.delete(id.value);
  }

  async count(): Promise<number> {
    return this.clients.size;
  }

  async findActiveClients(timeoutMs: number = 30000): Promise<Client[]> {
    const allClients = Array.from(this.clients.values());
    return allClients.filter(client => client.isActive(timeoutMs));
  }

  async cleanup(): Promise<void> {
    // Remove inactive clients older than 5 minutes
    const fiveMinutesAgo = Date.now() - (5 * 60 * 1000);
    const clientsToRemove: string[] = [];

    for (const [id, client] of this.clients) {
      if (client.lastActivity.getTime() < fiveMinutesAgo) {
        clientsToRemove.push(id);
      }
    }

    for (const id of clientsToRemove) {
      this.clients.delete(id);
    }
  }
}