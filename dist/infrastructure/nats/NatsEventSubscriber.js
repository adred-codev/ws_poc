import { connect, StringCodec } from 'nats';
const sc = StringCodec();
export class NatsEventSubscriber {
    config;
    messageService;
    nats = null;
    subscriptions = new Map();
    constructor(config, messageService) {
        this.config = config;
        this.messageService = messageService;
    }
    async connect() {
        try {
            console.log('📡 Connecting to NATS...', this.config.url);
            this.nats = await connect({
                servers: [this.config.url],
                reconnect: this.config.reconnect ?? true,
                maxReconnectAttempts: this.config.maxReconnectAttempts ?? 10,
                reconnectTimeWait: this.config.reconnectTimeWait ?? 2000
            });
            console.log('✅ Connected to NATS server');
        }
        catch (error) {
            console.error('❌ NATS connection failed:', error);
            throw error;
        }
    }
    async subscribeToOdinEvents() {
        if (!this.nats) {
            throw new Error('NATS connection not established');
        }
        // Subscribe to price updates
        await this.subscribeToSubject('odin.token.*.price', this.handlePriceUpdate.bind(this));
        // Subscribe to batch updates
        await this.subscribeToSubject('odin.token.batch.update', this.handleBatchUpdate.bind(this));
        // Subscribe to trade events
        await this.subscribeToSubject('odin.trades.*', this.handleTradeEvent.bind(this));
        console.log('📬 Subscribed to Odin subject hierarchy');
    }
    async subscribeToSubject(subject, handler) {
        if (!this.nats) {
            throw new Error('NATS connection not established');
        }
        const subscription = this.nats.subscribe(subject);
        this.subscriptions.set(subject, subscription);
        // Handle incoming messages (non-blocking)
        (async () => {
            for await (const msg of subscription) {
                try {
                    const data = JSON.parse(sc.decode(msg.data));
                    await handler(data);
                }
                catch (error) {
                    console.error(`❌ Error processing NATS message from ${subject}:`, error);
                }
            }
        })();
    }
    async handlePriceUpdate(data) {
        try {
            await this.messageService.publishPriceUpdate({
                tokenId: data.tokenId,
                price: data.price,
                priceChange24h: data.priceChange24h || 0,
                percentChange24h: data.percentChange24h || 0,
                volume24h: data.volume24h || 0,
                source: data.source || 'scheduler',
                nonce: data.nonce
            });
        }
        catch (error) {
            console.error('❌ Error handling price update:', error);
        }
    }
    async handleBatchUpdate(data) {
        try {
            if (data.updates && Array.isArray(data.updates)) {
                for (const update of data.updates) {
                    await this.messageService.publishPriceUpdate({
                        tokenId: update.tokenId,
                        price: update.price,
                        priceChange24h: update.priceChange24h || 0,
                        percentChange24h: update.percentChange24h || 0,
                        volume24h: update.volume24h || 0,
                        source: data.source || 'scheduler',
                        nonce: data.nonce
                    });
                }
            }
        }
        catch (error) {
            console.error('❌ Error handling batch update:', error);
        }
    }
    async handleTradeEvent(data) {
        try {
            // For trade events, we might want to publish a price update
            if (data.tokenId && data.price) {
                await this.messageService.publishPriceUpdate({
                    tokenId: data.tokenId,
                    price: data.price,
                    priceChange24h: 0, // Trade events might not have 24h data
                    percentChange24h: 0,
                    volume24h: data.amount || 0,
                    source: 'trade',
                    nonce: data.nonce
                });
            }
        }
        catch (error) {
            console.error('❌ Error handling trade event:', error);
        }
    }
    async disconnect() {
        // Close all subscriptions
        for (const subscription of this.subscriptions.values()) {
            subscription.unsubscribe();
        }
        this.subscriptions.clear();
        // Close NATS connection
        if (this.nats && !this.nats.isClosed()) {
            await this.nats.close();
        }
        console.log('✅ NATS connection closed');
    }
    isConnected() {
        return this.nats !== null && !this.nats.isClosed();
    }
}
//# sourceMappingURL=NatsEventSubscriber.js.map