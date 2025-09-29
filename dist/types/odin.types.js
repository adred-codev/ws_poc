// Message Types
export var MessageType;
(function (MessageType) {
    MessageType["PRICE_UPDATE"] = "price:update";
    MessageType["VOLUME_UPDATE"] = "volume:update";
    MessageType["TRADE_EXECUTED"] = "trade:executed";
    MessageType["BATCH_UPDATE"] = "batch:update";
    MessageType["MARKET_STATS"] = "market:stats";
    MessageType["HOLDER_UPDATE"] = "holder:update";
    MessageType["CONNECTION_ESTABLISHED"] = "connection:established";
    MessageType["HEARTBEAT"] = "heartbeat";
    MessageType["SUBSCRIPTION_ACK"] = "subscription:ack";
    MessageType["PING"] = "ping";
    MessageType["PONG"] = "pong";
    MessageType["SUBSCRIBE"] = "subscribe";
    MessageType["UNSUBSCRIBE"] = "unsubscribe";
})(MessageType || (MessageType = {}));
//# sourceMappingURL=odin.types.js.map