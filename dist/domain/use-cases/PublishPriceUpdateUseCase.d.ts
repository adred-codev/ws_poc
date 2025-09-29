import { MessageSource } from '../entities/Message';
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
export declare class PublishPriceUpdateUseCase {
    private readonly messageRepository;
    private readonly clientRepository;
    constructor(messageRepository: IMessageRepository, clientRepository: IClientRepository);
    execute(request: PublishPriceUpdateRequest): Promise<PublishPriceUpdateResponse>;
    private checkForDuplicates;
    private markNonceAsSeen;
}
//# sourceMappingURL=PublishPriceUpdateUseCase.d.ts.map