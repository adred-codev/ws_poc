import { MessageType } from '../../types/odin.types';
import { Nonce } from '../value-objects/Nonce';
import { TokenId } from '../value-objects/TokenId';
import { Price } from '../value-objects/Price';
export declare abstract class Message {
    protected readonly _type: MessageType;
    protected readonly _timestamp: Date;
    protected readonly _nonce: Nonce;
    constructor(type: MessageType, timestamp?: Date, nonce?: Nonce);
    get type(): MessageType;
    get timestamp(): Date;
    get nonce(): Nonce;
    abstract toJSON(): Record<string, any>;
}
export type MessageSource = 'trade' | 'scheduler';
export declare class PriceUpdateMessage extends Message {
    private readonly _tokenId;
    private readonly _price;
    private readonly _priceChange24h;
    private readonly _percentChange24h;
    private readonly _volume24h;
    private readonly _source;
    constructor(_tokenId: TokenId, _price: Price, _priceChange24h: number, _percentChange24h: number, _volume24h: number, _source: MessageSource, timestamp?: Date, nonce?: Nonce);
    get tokenId(): TokenId;
    get price(): Price;
    get priceChange24h(): number;
    get percentChange24h(): number;
    get volume24h(): number;
    get source(): MessageSource;
    toJSON(): Record<string, any>;
}
export declare class TradeExecutedMessage extends Message {
    private readonly _tradeId;
    private readonly _tokenId;
    private readonly _userId;
    private readonly _side;
    private readonly _price;
    private readonly _amount;
    constructor(_tradeId: string, _tokenId: TokenId, _userId: string, _side: 'buy' | 'sell', _price: Price, _amount: number, timestamp?: Date, nonce?: Nonce);
    get tradeId(): string;
    get tokenId(): TokenId;
    get userId(): string;
    get side(): 'buy' | 'sell';
    get price(): Price;
    get amount(): number;
    toJSON(): Record<string, any>;
}
export declare class HeartbeatMessage extends Message {
    private readonly _connectedClients;
    private readonly _idleTime?;
    constructor(_connectedClients: number, _idleTime?: number | undefined, timestamp?: Date, nonce?: Nonce);
    get connectedClients(): number;
    get idleTime(): number | undefined;
    toJSON(): Record<string, any>;
}
//# sourceMappingURL=Message.d.ts.map