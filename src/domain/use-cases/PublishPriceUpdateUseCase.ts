import { PriceUpdateMessage, MessageSource } from '../entities/Message';
import { TokenId } from '../value-objects/TokenId';
import { Price } from '../value-objects/Price';
import { Nonce } from '../value-objects/Nonce';
import { IMessageRepository } from '../repositories/IMessageRepository';
import { IClientRepository } from '../repositories/IClientRepository';

export interface PublishPriceUpdateRequest {
  tokenId: string;
  price: number;
  priceChange24h: number;
  percentChange24h: number;
  volume24h: number;
  source: MessageSource;
  nonce?: string;
}

export interface PublishPriceUpdateResponse {
  messagesSent: number;
  duplicatesDropped: number;
  success: boolean;
}

export class PublishPriceUpdateUseCase {
  constructor(
    private readonly messageRepository: IMessageRepository,
    private readonly clientRepository: IClientRepository
  ) {}

  async execute(request: PublishPriceUpdateRequest): Promise<PublishPriceUpdateResponse> {
    try {
      // Create domain objects
      const tokenId = TokenId.fromString(request.tokenId);
      const price = Price.fromNumber(request.price);
      const nonce = request.nonce ? Nonce.fromString(request.nonce) : Nonce.generate();

      // Create price update message
      const message = new PriceUpdateMessage(
        tokenId,
        price,
        request.priceChange24h,
        request.percentChange24h,
        request.volume24h,
        request.source,
        new Date(),
        nonce
      );

      // Check for duplicates across all clients
      const duplicatesDropped = await this.checkForDuplicates(nonce);

      if (duplicatesDropped > 0) {
        return {
          messagesSent: 0,
          duplicatesDropped,
          success: true
        };
      }

      // Mark nonce as seen for all clients
      await this.markNonceAsSeen(nonce);

      // Broadcast to all clients
      const messagesSent = await this.messageRepository.broadcastToAllClients(message);

      return {
        messagesSent,
        duplicatesDropped: 0,
        success: true
      };
    } catch (error) {
      throw new Error(`Failed to publish price update: ${error}`);
    }
  }

  private async checkForDuplicates(nonce: Nonce): Promise<number> {
    const clients = await this.clientRepository.findAll();
    let duplicateCount = 0;

    for (const client of clients) {
      if (client.hasSeenNonce(nonce)) {
        duplicateCount++;
      }
    }

    return duplicateCount;
  }

  private async markNonceAsSeen(nonce: Nonce): Promise<void> {
    const clients = await this.clientRepository.findAll();

    for (const client of clients) {
      if (!client.hasSeenNonce(nonce)) {
        client.addSeenNonce(nonce);
        await this.clientRepository.save(client);
      }
    }
  }
}