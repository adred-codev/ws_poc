import { connect, NatsConnection, StringCodec, Subscription } from 'nats';
import { MessageService } from '../../application/services/MessageService';
import { MessageSource } from '../../domain/entities/Message';

const sc = StringCodec();

export interface NatsConfig {
  url: string;
  reconnect?: boolean;
  maxReconnectAttempts?: number;
  reconnectTimeWait?: number;
}

export class NatsEventSubscriber {
  private nats: NatsConnection | null = null;
  private subscriptions: Map<string, Subscription> = new Map();

  constructor(
    private readonly config: NatsConfig,
    private readonly messageService: MessageService
  ) {}

  async connect(): Promise<void> {
    try {
      console.log('📡 Connecting to NATS...', this.config.url);

      this.nats = await connect({
        servers: [this.config.url],
        reconnect: this.config.reconnect ?? true,
        maxReconnectAttempts: this.config.maxReconnectAttempts ?? 10,
        reconnectTimeWait: this.config.reconnectTimeWait ?? 2000
      });

      console.log('✅ Connected to NATS server');
    } catch (error) {
      console.error('❌ NATS connection failed:', error);
      throw error;
    }
  }

  async subscribeToOdinEvents(): Promise<void> {
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

  private async subscribeToSubject(subject: string, handler: (data: any) => Promise<void>): Promise<void> {
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
        } catch (error) {
          console.error(`❌ Error processing NATS message from ${subject}:`, error);
        }
      }
    })();
  }

  private async handlePriceUpdate(data: any): Promise<void> {
    try {
      await this.messageService.publishPriceUpdate({
        tokenId: data.tokenId,
        price: data.price,
        priceChange24h: data.priceChange24h || 0,
        percentChange24h: data.percentChange24h || 0,
        volume24h: data.volume24h || 0,
        source: data.source as MessageSource || 'scheduler',
        nonce: data.nonce
      });
    } catch (error) {
      console.error('❌ Error handling price update:', error);
    }
  }

  private async handleBatchUpdate(data: any): Promise<void> {
    try {
      if (data.updates && Array.isArray(data.updates)) {
        for (const update of data.updates) {
          await this.messageService.publishPriceUpdate({
            tokenId: update.tokenId,
            price: update.price,
            priceChange24h: update.priceChange24h || 0,
            percentChange24h: update.percentChange24h || 0,
            volume24h: update.volume24h || 0,
            source: data.source as MessageSource || 'scheduler',
            nonce: data.nonce
          });
        }
      }
    } catch (error) {
      console.error('❌ Error handling batch update:', error);
    }
  }

  private async handleTradeEvent(data: any): Promise<void> {
    try {
      // For trade events, we might want to publish a price update
      if (data.tokenId && data.price) {
        await this.messageService.publishPriceUpdate({
          tokenId: data.tokenId,
          price: data.price,
          priceChange24h: 0, // Trade events might not have 24h data
          percentChange24h: 0,
          volume24h: data.amount || 0,
          source: 'trade' as MessageSource,
          nonce: data.nonce
        });
      }
    } catch (error) {
      console.error('❌ Error handling trade event:', error);
    }
  }

  async disconnect(): Promise<void> {
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

  isConnected(): boolean {
    return this.nats !== null && !this.nats.isClosed();
  }
}