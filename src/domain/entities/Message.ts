import { MessageType } from '../../types/odin.types.js';
import { Nonce } from '../value-objects/Nonce.js';
import { TokenId } from '../value-objects/TokenId.js';
import { Price } from '../value-objects/Price.js';

export abstract class Message {
  protected readonly _type: MessageType;
  protected readonly _timestamp: Date;
  protected readonly _nonce: Nonce;

  constructor(type: MessageType, timestamp: Date = new Date(), nonce?: Nonce) {
    this._type = type;
    this._timestamp = timestamp;
    this._nonce = nonce || Nonce.generate();
  }

  get type(): MessageType {
    return this._type;
  }

  get timestamp(): Date {
    return this._timestamp;
  }

  get nonce(): Nonce {
    return this._nonce;
  }

  abstract toJSON(): Record<string, any>;
}

export type MessageSource = 'trade' | 'scheduler';

export class PriceUpdateMessage extends Message {
  constructor(
    private readonly _tokenId: TokenId,
    private readonly _price: Price,
    private readonly _priceChange24h: number,
    private readonly _percentChange24h: number,
    private readonly _volume24h: number,
    private readonly _source: MessageSource,
    timestamp?: Date,
    nonce?: Nonce
  ) {
    super(MessageType.PRICE_UPDATE, timestamp, nonce);
  }

  get tokenId(): TokenId {
    return this._tokenId;
  }

  get price(): Price {
    return this._price;
  }

  get priceChange24h(): number {
    return this._priceChange24h;
  }

  get percentChange24h(): number {
    return this._percentChange24h;
  }

  get volume24h(): number {
    return this._volume24h;
  }

  get source(): MessageSource {
    return this._source;
  }

  toJSON(): Record<string, any> {
    return {
      type: this._type,
      tokenId: this._tokenId.value,
      price: this._price.value,
      priceChange24h: this._priceChange24h,
      percentChange24h: this._percentChange24h,
      volume24h: this._volume24h,
      source: this._source,
      timestamp: this._timestamp.getTime(),
      nonce: this._nonce.value
    };
  }
}

export class TradeExecutedMessage extends Message {
  constructor(
    private readonly _tradeId: string,
    private readonly _tokenId: TokenId,
    private readonly _userId: string,
    private readonly _side: 'buy' | 'sell',
    private readonly _price: Price,
    private readonly _amount: number,
    timestamp?: Date,
    nonce?: Nonce
  ) {
    super(MessageType.TRADE_EXECUTED, timestamp, nonce);
  }

  get tradeId(): string {
    return this._tradeId;
  }

  get tokenId(): TokenId {
    return this._tokenId;
  }

  get userId(): string {
    return this._userId;
  }

  get side(): 'buy' | 'sell' {
    return this._side;
  }

  get price(): Price {
    return this._price;
  }

  get amount(): number {
    return this._amount;
  }

  toJSON(): Record<string, any> {
    return {
      type: this._type,
      tradeId: this._tradeId,
      tokenId: this._tokenId.value,
      userId: this._userId,
      side: this._side,
      price: this._price.value,
      amount: this._amount,
      timestamp: this._timestamp.getTime(),
      nonce: this._nonce.value
    };
  }
}

export class HeartbeatMessage extends Message {
  constructor(
    private readonly _connectedClients: number,
    private readonly _idleTime?: number,
    timestamp?: Date,
    nonce?: Nonce
  ) {
    super(MessageType.HEARTBEAT, timestamp, nonce);
  }

  get connectedClients(): number {
    return this._connectedClients;
  }

  get idleTime(): number | undefined {
    return this._idleTime;
  }

  toJSON(): Record<string, any> {
    return {
      type: this._type,
      connectedClients: this._connectedClients,
      idleTime: this._idleTime,
      timestamp: this._timestamp.getTime(),
      nonce: this._nonce.value
    };
  }
}