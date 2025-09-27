import { Client } from '../entities/Client';
import { ClientId } from '../value-objects/ClientId';

export interface IClientRepository {
  save(client: Client): Promise<void>;
  findById(id: ClientId): Promise<Client | null>;
  findAll(): Promise<Client[]>;
  remove(id: ClientId): Promise<void>;
  count(): Promise<number>;
  findActiveClients(timeoutMs?: number): Promise<Client[]>;
  cleanup(): Promise<void>;
}