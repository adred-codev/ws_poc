import { Client } from '../entities/Client';
import { ClientId } from '../value-objects/ClientId';
import { IClientRepository } from '../repositories/IClientRepository';
import { IMessageRepository } from '../repositories/IMessageRepository';
import { MessageType } from '../../types/odin.types';
import { Message } from '../entities/Message';

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

export class ConnectClientUseCase {
  constructor(
    private readonly clientRepository: IClientRepository,
    private readonly messageRepository: IMessageRepository
  ) {}

  async execute(request: ConnectClientRequest): Promise<ConnectClientResponse> {
    try {
      // Generate unique client ID
      const clientId = ClientId.generate();

      // Create client entity
      const client = new Client(
        clientId,
        request.ip,
        request.userAgent
      );

      // Save client
      await this.clientRepository.save(client);

      // Send welcome message
      await this.sendWelcomeMessage(clientId);

      return {
        clientId,
        success: true,
        message: 'Connected to Odin WebSocket server'
      };
    } catch (error) {
      throw new Error(`Failed to connect client: ${error}`);
    }
  }

  private async sendWelcomeMessage(clientId: ClientId): Promise<void> {
    const welcomeMessage = new WelcomeMessage(clientId);
    await this.messageRepository.publishToClient(clientId, welcomeMessage);
  }
}

class WelcomeMessage extends Message {
  constructor(
    private readonly _clientId: ClientId,
    timestamp?: Date
  ) {
    super(MessageType.CONNECTION_ESTABLISHED, timestamp);
  }

  get clientId(): ClientId {
    return this._clientId;
  }

  toJSON(): Record<string, any> {
    return {
      type: this._type,
      clientId: this._clientId.value,
      timestamp: this._timestamp.getTime(),
      message: 'Connected to Odin WebSocket server',
      nonce: this._nonce.value
    };
  }
}