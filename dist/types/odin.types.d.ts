export declare enum MessageType {
    PRICE_UPDATE = "price:update",
    VOLUME_UPDATE = "volume:update",
    TRADE_EXECUTED = "trade:executed",
    BATCH_UPDATE = "batch:update",
    MARKET_STATS = "market:stats",
    HOLDER_UPDATE = "holder:update",
    CONNECTION_ESTABLISHED = "connection:established",
    HEARTBEAT = "heartbeat",
    SUBSCRIPTION_ACK = "subscription:ack",
    PING = "ping",
    PONG = "pong",
    SUBSCRIBE = "subscribe",
    UNSUBSCRIBE = "unsubscribe"
}
export interface BaseMessage {
    type: MessageType;
    timestamp: number;
    nonce: string;
}
export interface PriceUpdateMessage extends BaseMessage {
    type: MessageType.PRICE_UPDATE;
    tokenId: string;
    price: number;
    priceChange24h: number;
    percentChange24h: number;
    volume24h: number;
    source: 'trade' | 'scheduler';
}
export interface TradeExecutedMessage extends BaseMessage {
    type: MessageType.TRADE_EXECUTED;
    tradeId: string;
    tokenId: string;
    userId: string;
    side: 'buy' | 'sell';
    price: number;
    amount: number;
}
export interface VolumeUpdateMessage extends BaseMessage {
    type: MessageType.VOLUME_UPDATE;
    tokenId: string;
    volume24h: number;
    trades24h: number;
    source: 'scheduler';
}
export interface BatchUpdateMessage extends BaseMessage {
    type: MessageType.BATCH_UPDATE;
    updates: Array<{
        tokenId: string;
        price: number;
        priceChange24h: number;
        percentChange24h: number;
        volume24h: number;
    }>;
    source: 'scheduler';
}
export interface MarketStatsMessage extends BaseMessage {
    type: MessageType.MARKET_STATS;
    totalMarketCap: number;
    totalVolume24h: number;
    totalTrades24h: number;
    activeTokens: number;
    topGainers: Array<{
        tokenId: string;
        change: number;
    }>;
    topLosers: Array<{
        tokenId: string;
        change: number;
    }>;
}
export interface HolderUpdateMessage extends BaseMessage {
    type: MessageType.HOLDER_UPDATE;
    tokenId: string;
    holders: number;
    change24h: number;
    source: 'scheduler';
}
export interface ConnectionEstablishedMessage extends BaseMessage {
    type: MessageType.CONNECTION_ESTABLISHED;
    clientId: string;
    message: string;
    rolloutPercentage?: number;
    features?: {
        deduplication: boolean;
        sourceTracking: boolean;
        metricsEnabled: boolean;
    };
}
export interface HeartbeatMessage extends BaseMessage {
    type: MessageType.HEARTBEAT;
    connectedClients: number;
    idleTime?: number;
}
export interface PingMessage extends BaseMessage {
    type: MessageType.PING;
}
export interface PongMessage extends BaseMessage {
    type: MessageType.PONG;
    originalTimestamp?: number;
    latency?: number;
}
export interface SubscribeMessage extends BaseMessage {
    type: MessageType.SUBSCRIBE;
    tokens: string[];
}
export interface SubscriptionAckMessage extends BaseMessage {
    type: MessageType.SUBSCRIPTION_ACK;
    subscriptions: string[];
}
export type OdinMessage = PriceUpdateMessage | TradeExecutedMessage | VolumeUpdateMessage | BatchUpdateMessage | MarketStatsMessage | HolderUpdateMessage | ConnectionEstablishedMessage | HeartbeatMessage | PingMessage | PongMessage | SubscribeMessage | SubscriptionAckMessage;
export interface ClientInfo {
    id: string;
    connectedAt: number;
    ip: string;
    userAgent: string;
    messageCount: number;
    lastActivity: number;
    seenNonces: Set<string>;
    heartbeatInterval?: NodeJS.Timeout;
}
export interface ConnectionState {
    subscriptions: Set<string>;
    preferences: Record<string, any>;
}
export interface TokenData {
    price: number;
    volume24h: number;
    holders: number;
    priceChange24h: number;
    percentChange24h: number;
    trades24h: number;
}
export interface ServerMetrics {
    messagesPublished: number;
    messagesDelivered: number;
    connectionCount: number;
    duplicatesDropped: number;
    reconnectionCount: number;
    apiRequestsSaved: number;
    bandwidthSaved: number;
    latencySum: number;
    latencyCount: number;
    peakLatency: number;
    errors: number;
    startTime: number;
}
export interface NatsConfig {
    url: string;
    cluster: string;
    jetstream: {
        enabled: boolean;
        stream: string;
        retention: string;
        maxAge: number;
        maxMsgs: number;
        dedupWindow: number;
    };
}
export interface ServerConfig {
    wsPort: number;
    httpPort: number;
}
export interface RedisConfig {
    enabled: boolean;
    url: string;
    ttl: number;
}
export interface JwtConfig {
    secret: string;
    expiry: number;
}
export interface SimulationConfig {
    tokens: string[];
    tradeFrequency: number;
    enableScheduler: boolean;
    enableTradeSimulator: boolean;
}
export interface OdinConfig {
    nats: NatsConfig;
    server: ServerConfig;
    redis: RedisConfig;
    jwt: JwtConfig;
    simulation: SimulationConfig;
    env: string;
    logLevel: string;
}
export interface UpdateFrequencies {
    TOKEN_PRICES: number;
    PRICE_DELTAS: number;
    BTC_PRICE: number;
    TOKEN_VOLUMES: number;
    HOLDER_COUNTS: number;
    STATISTICS: number;
    USER_TRADES: number;
}
export interface NatsSubjects {
    tokenPrice: (tokenId: string) => string;
    tokenVolume: (tokenId: string) => string;
    tokenHolders: (tokenId: string) => string;
    batchUpdate: string;
    trades: (tokenId: string) => string;
    userTrades: (userId: string) => string;
    marketStats: string;
    systemHealth: string;
    connectionState: string;
}
export interface TradeRequest {
    tokenId: string;
    userId: string;
    side: 'buy' | 'sell';
    amount: number;
}
export interface TradeResponse {
    success: boolean;
    tradeId: string;
    executionPrice: number;
}
export interface AuthTokenRequest {
    userId?: string;
}
export interface AuthTokenResponse {
    token: string;
    userId: string;
}
//# sourceMappingURL=odin.types.d.ts.map