import { MessageType } from '../../types/odin.types';
import { Nonce } from '../value-objects/Nonce';
export class Message {
    _type;
    _timestamp;
    _nonce;
    constructor(type, timestamp = new Date(), nonce) {
        this._type = type;
        this._timestamp = timestamp;
        this._nonce = nonce || Nonce.generate();
    }
    get type() {
        return this._type;
    }
    get timestamp() {
        return this._timestamp;
    }
    get nonce() {
        return this._nonce;
    }
}
export class PriceUpdateMessage extends Message {
    _tokenId;
    _price;
    _priceChange24h;
    _percentChange24h;
    _volume24h;
    _source;
    constructor(_tokenId, _price, _priceChange24h, _percentChange24h, _volume24h, _source, timestamp, nonce) {
        super(MessageType.PRICE_UPDATE, timestamp, nonce);
        this._tokenId = _tokenId;
        this._price = _price;
        this._priceChange24h = _priceChange24h;
        this._percentChange24h = _percentChange24h;
        this._volume24h = _volume24h;
        this._source = _source;
    }
    get tokenId() {
        return this._tokenId;
    }
    get price() {
        return this._price;
    }
    get priceChange24h() {
        return this._priceChange24h;
    }
    get percentChange24h() {
        return this._percentChange24h;
    }
    get volume24h() {
        return this._volume24h;
    }
    get source() {
        return this._source;
    }
    toJSON() {
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
    _tradeId;
    _tokenId;
    _userId;
    _side;
    _price;
    _amount;
    constructor(_tradeId, _tokenId, _userId, _side, _price, _amount, timestamp, nonce) {
        super(MessageType.TRADE_EXECUTED, timestamp, nonce);
        this._tradeId = _tradeId;
        this._tokenId = _tokenId;
        this._userId = _userId;
        this._side = _side;
        this._price = _price;
        this._amount = _amount;
    }
    get tradeId() {
        return this._tradeId;
    }
    get tokenId() {
        return this._tokenId;
    }
    get userId() {
        return this._userId;
    }
    get side() {
        return this._side;
    }
    get price() {
        return this._price;
    }
    get amount() {
        return this._amount;
    }
    toJSON() {
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
    _connectedClients;
    _idleTime;
    constructor(_connectedClients, _idleTime, timestamp, nonce) {
        super(MessageType.HEARTBEAT, timestamp, nonce);
        this._connectedClients = _connectedClients;
        this._idleTime = _idleTime;
    }
    get connectedClients() {
        return this._connectedClients;
    }
    get idleTime() {
        return this._idleTime;
    }
    toJSON() {
        return {
            type: this._type,
            connectedClients: this._connectedClients,
            idleTime: this._idleTime,
            timestamp: this._timestamp.getTime(),
            nonce: this._nonce.value
        };
    }
}
//# sourceMappingURL=Message.js.map