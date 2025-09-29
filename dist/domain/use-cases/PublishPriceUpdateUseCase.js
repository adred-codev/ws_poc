import { PriceUpdateMessage } from '../entities/Message';
import { TokenId } from '../value-objects/TokenId';
import { Price } from '../value-objects/Price';
import { Nonce } from '../value-objects/Nonce';
export class PublishPriceUpdateUseCase {
    messageRepository;
    clientRepository;
    constructor(messageRepository, clientRepository) {
        this.messageRepository = messageRepository;
        this.clientRepository = clientRepository;
    }
    async execute(request) {
        try {
            // Create domain objects
            const tokenId = TokenId.fromString(request.tokenId);
            const price = Price.fromNumber(request.price);
            const nonce = request.nonce ? Nonce.fromString(request.nonce) : Nonce.generate();
            // Create price update message
            const message = new PriceUpdateMessage(tokenId, price, request.priceChange24h, request.percentChange24h, request.volume24h, request.source, new Date(), nonce);
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
        }
        catch (error) {
            throw new Error(`Failed to publish price update: ${error}`);
        }
    }
    async checkForDuplicates(nonce) {
        const clients = await this.clientRepository.findAll();
        let duplicateCount = 0;
        for (const client of clients) {
            if (client.hasSeenNonce(nonce)) {
                duplicateCount++;
            }
        }
        return duplicateCount;
    }
    async markNonceAsSeen(nonce) {
        const clients = await this.clientRepository.findAll();
        for (const client of clients) {
            if (!client.hasSeenNonce(nonce)) {
                client.addSeenNonce(nonce);
                await this.clientRepository.save(client);
            }
        }
    }
}
//# sourceMappingURL=PublishPriceUpdateUseCase.js.map