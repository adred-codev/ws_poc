import { Client } from '../../domain/entities/Client.js';
import { ClientId } from '../../domain/value-objects/ClientId.js';
import { IClientRepository } from '../../domain/repositories/IClientRepository.js';
import { ConnectClientUseCase, ConnectClientRequest, ConnectClientResponse } from '../../domain/use-cases/ConnectClientUseCase.js';
import { IMessageRepository } from '../../domain/repositories/IMessageRepository.js';

export interface ConnectionMetrics {
  totalConnections: number;
  currentConnections: number;
  averageConnectionDuration: number;
  activeClients: number;
}

export class ConnectionService {
  private readonly connectClientUseCase: ConnectClientUseCase;

  constructor(
    private readonly clientRepository: IClientRepository,
    private readonly messageRepository: IMessageRepository
  ) {
    this.connectClientUseCase = new ConnectClientUseCase(
      clientRepository,
      messageRepository
    );
  }

  async connectClient(request: ConnectClientRequest): Promise<ConnectClientResponse> {
    return await this.connectClientUseCase.execute(request);
  }

  async disconnectClient(clientId: ClientId): Promise<void> {
    await this.clientRepository.remove(clientId);
  }

  async findClient(clientId: ClientId): Promise<Client | null> {
    return await this.clientRepository.findById(clientId);
  }

  async getConnectionMetrics(): Promise<ConnectionMetrics> {
    const allClients = await this.clientRepository.findAll();
    const activeClients = await this.clientRepository.findActiveClients();

    const totalDuration = allClients.reduce(
      (sum, client) => sum + client.connectionDuration,
      0
    );
    const averageConnectionDuration = allClients.length > 0
      ? totalDuration / allClients.length
      : 0;

    return {
      totalConnections: allClients.length,
      currentConnections: allClients.length,
      averageConnectionDuration,
      activeClients: activeClients.length
    };
  }

  async updateClientActivity(clientId: ClientId): Promise<void> {
    const client = await this.clientRepository.findById(clientId);
    if (client) {
      client.updateActivity();
      await this.clientRepository.save(client);
    }
  }

  async cleanupInactiveClients(timeoutMs: number = 60000): Promise<number> {
    const allClients = await this.clientRepository.findAll();
    let cleanedUp = 0;

    for (const client of allClients) {
      if (!client.isActive(timeoutMs)) {
        await this.clientRepository.remove(client.id);
        cleanedUp++;
      }
    }

    return cleanedUp;
  }

  async getAllClients(): Promise<Client[]> {
    return await this.clientRepository.findAll();
  }

  async getActiveClients(timeoutMs?: number): Promise<Client[]> {
    return await this.clientRepository.findActiveClients(timeoutMs);
  }
}