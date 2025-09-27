import { connect, NatsConnection, StringCodec } from 'nats';
import crypto from 'crypto';
import {
  config,
  subjects,
  updateFrequencies
} from './config/odin.config';
import {
  TokenData,
  PriceUpdateMessage,
  TradeExecutedMessage,
  VolumeUpdateMessage,
  BatchUpdateMessage,
  MarketStatsMessage,
  HolderUpdateMessage
} from './types/odin.types';
import { MessageType } from './types/odin.types';

const sc = StringCodec();

interface MarketStats {
  totalMarketCap: number;
  totalVolume24h: number;
  totalTrades24h: number;
  activeTokens: number;
}

class OdinPublisher {
  private nats: NatsConnection | null = null;
  private tokenData: Map<string, TokenData> = new Map();
  private marketStats: MarketStats = {
    totalMarketCap: 0,
    totalVolume24h: 0,
    totalTrades24h: 0,
    activeTokens: 0
  };
  private intervals: Map<string, NodeJS.Timeout> = new Map();
  private publishCount = 0;
  private startTime = Date.now();

  async initialize(): Promise<void> {
    await this.connectToNATS();
    this.initializeTokenData();
    this.startScheduledUpdates();
    this.startTradeSimulator();
    this.startMetricsReporting();
  }

  private async connectToNATS(): Promise<void> {
    try {
      console.log('📡 Connecting to NATS...', config.nats.url);
      this.nats = await connect({
        servers: [config.nats.url],
        reconnect: true,
        maxReconnectAttempts: 10
      });
      console.log('✅ Connected to NATS server');
    } catch (error) {
      console.error('❌ NATS connection failed:', error);
      process.exit(1);
    }
  }

  private initializeTokenData(): void {
    const basePrices: Record<string, number> = {
      BTC: 45000,
      ETH: 2500,
      ODIN: 0.15,
      SOL: 100,
      DOGE: 0.08,
      USDT: 1.0,
      BNB: 300,
      XRP: 0.5,
      ADA: 0.4,
      MATIC: 0.8
    };

    config.simulation.tokens.forEach(token => {
      this.tokenData.set(token, {
        price: basePrices[token] || Math.random() * 100,
        volume24h: Math.random() * 1000000000,
        holders: Math.floor(Math.random() * 100000),
        priceChange24h: 0,
        percentChange24h: 0,
        trades24h: Math.floor(Math.random() * 10000)
      });
    });

    console.log('💰 Initialized token data for:', Array.from(this.tokenData.keys()));
  }

  private generateNonce(): string {
    return `${Date.now()}-${crypto.randomBytes(8).toString('hex')}`;
  }

  private startScheduledUpdates(): void {
    // Token prices - every 1 minute
    this.intervals.set('prices', setInterval(() => {
      this.publishBatchPriceUpdate();
    }, updateFrequencies.TOKEN_PRICES));

    // Token volumes - every 10 minutes
    this.intervals.set('volumes', setInterval(() => {
      this.publishVolumeUpdates();
    }, updateFrequencies.TOKEN_VOLUMES));

    // Holder counts - every 10 minutes
    this.intervals.set('holders', setInterval(() => {
      this.publishHolderUpdates();
    }, updateFrequencies.HOLDER_COUNTS));

    // Market statistics - every 60 minutes
    this.intervals.set('stats', setInterval(() => {
      this.publishMarketStats();
    }, updateFrequencies.STATISTICS));

    // BTC special update - every 5 minutes
    this.intervals.set('btc', setInterval(() => {
      this.publishSpecialBTCUpdate();
    }, updateFrequencies.BTC_PRICE));

    console.log('⏰ Started scheduled updates with Odin frequencies');
  }

  private startTradeSimulator(): void {
    if (!config.simulation.enableTradeSimulator) return;

    setInterval(() => {
      const tokens = Array.from(this.tokenData.keys());
      const randomToken = tokens[Math.floor(Math.random() * tokens.length)];
      this.simulateTrade(randomToken);
    }, config.simulation.tradeFrequency);

    console.log('🤖 Trade simulator started');
  }

  private async simulateTrade(tokenId: string): Promise<void> {
    const tokenInfo = this.tokenData.get(tokenId);
    if (!tokenInfo) return;

    const side: 'buy' | 'sell' = Math.random() > 0.5 ? 'buy' : 'sell';
    const priceImpact = (Math.random() - 0.5) * 0.02; // ±1% impact

    // Update price based on trade
    const oldPrice = tokenInfo.price;
    tokenInfo.price = tokenInfo.price * (1 + priceImpact);
    tokenInfo.volume24h += tokenInfo.price * Math.random() * 1000;
    tokenInfo.trades24h++;

    // Create trade message
    const tradeMessage: TradeExecutedMessage = {
      type: MessageType.TRADE_EXECUTED,
      tradeId: crypto.randomBytes(16).toString('hex'),
      tokenId,
      userId: `user-${Math.floor(Math.random() * 1000)}`,
      side,
      price: tokenInfo.price,
      amount: Math.random() * 1000,
      timestamp: Date.now(),
      nonce: this.generateNonce()
    };

    // Publish trade event
    await this.publish(subjects.trades(tokenId), tradeMessage);

    // Immediately publish price update from trade
    const priceUpdate: PriceUpdateMessage = {
      type: MessageType.PRICE_UPDATE,
      tokenId,
      price: tokenInfo.price,
      priceChange24h: tokenInfo.price - oldPrice,
      percentChange24h: ((tokenInfo.price - oldPrice) / oldPrice) * 100,
      volume24h: tokenInfo.volume24h,
      timestamp: Date.now(),
      source: 'trade', // This is from a trade!
      nonce: this.generateNonce()
    };

    await this.publish(subjects.tokenPrice(tokenId), priceUpdate);
    console.log(`🔄 Trade executed: ${tokenId} ${side} at $${tokenInfo.price.toFixed(2)} (source: trade)`);
  }

  private async publishBatchPriceUpdate(): Promise<void> {
    const updates: Array<{
      tokenId: string;
      price: number;
      priceChange24h: number;
      percentChange24h: number;
      volume24h: number;
    }> = [];

    for (const [tokenId, data] of this.tokenData) {
      // Simulate price movement
      const change = (Math.random() - 0.5) * 0.01; // ±0.5% change
      data.price = data.price * (1 + change);
      data.priceChange24h = data.price * change;
      data.percentChange24h = change * 100;

      updates.push({
        tokenId,
        price: data.price,
        priceChange24h: data.priceChange24h,
        percentChange24h: data.percentChange24h,
        volume24h: data.volume24h
      });
    }

    const batchMessage: BatchUpdateMessage = {
      type: MessageType.BATCH_UPDATE,
      updates,
      source: 'scheduler', // This is from scheduler!
      timestamp: Date.now(),
      nonce: this.generateNonce()
    };

    await this.publish(subjects.batchUpdate, batchMessage);
    console.log(`📦 Batch update published: ${updates.length} tokens (source: scheduler)`);
  }

  private async publishVolumeUpdates(): Promise<void> {
    for (const [tokenId, data] of this.tokenData) {
      // Simulate volume changes
      data.volume24h = data.volume24h * (0.8 + Math.random() * 0.4);

      const volumeMessage: VolumeUpdateMessage = {
        type: MessageType.VOLUME_UPDATE,
        tokenId,
        volume24h: data.volume24h,
        trades24h: data.trades24h,
        timestamp: Date.now(),
        source: 'scheduler',
        nonce: this.generateNonce()
      };

      await this.publish(subjects.tokenVolume(tokenId), volumeMessage);
    }
    console.log('📊 Volume updates published for all tokens');
  }

  private async publishHolderUpdates(): Promise<void> {
    for (const [tokenId, data] of this.tokenData) {
      // Simulate holder count changes
      const change = Math.floor((Math.random() - 0.5) * 100);
      data.holders = Math.max(1, data.holders + change);

      const holderMessage: HolderUpdateMessage = {
        type: MessageType.HOLDER_UPDATE,
        tokenId,
        holders: data.holders,
        change24h: change,
        timestamp: Date.now(),
        source: 'scheduler',
        nonce: this.generateNonce()
      };

      await this.publish(subjects.tokenHolders(tokenId), holderMessage);
    }
    console.log('👥 Holder count updates published');
  }

  private async publishSpecialBTCUpdate(): Promise<void> {
    const btcData = this.tokenData.get('BTC');
    if (!btcData) return;

    // Special BTC price update (simulating external API)
    btcData.price = btcData.price * (0.99 + Math.random() * 0.02);

    const btcMessage: PriceUpdateMessage & { special: string } = {
      type: MessageType.PRICE_UPDATE,
      tokenId: 'BTC',
      price: btcData.price,
      priceChange24h: btcData.priceChange24h,
      percentChange24h: btcData.percentChange24h,
      volume24h: btcData.volume24h,
      timestamp: Date.now(),
      source: 'scheduler',
      nonce: this.generateNonce(),
      special: 'btc-5min-update'
    };

    await this.publish(subjects.tokenPrice('BTC'), btcMessage);
    console.log(`₿ Special BTC update: $${btcData.price.toFixed(2)}`);
  }

  private async publishMarketStats(): Promise<void> {
    // Calculate market statistics
    let totalMarketCap = 0;
    let totalVolume24h = 0;
    let totalTrades24h = 0;
    const priceChanges: Array<{ tokenId: string; change: number }> = [];

    for (const [tokenId, data] of this.tokenData) {
      totalMarketCap += data.price * 1000000; // Simulated supply
      totalVolume24h += data.volume24h;
      totalTrades24h += data.trades24h;
      priceChanges.push({ tokenId, change: data.percentChange24h });
    }

    // Sort for top gainers/losers
    priceChanges.sort((a, b) => b.change - a.change);
    const topGainers = priceChanges.slice(0, 3);
    const topLosers = priceChanges.slice(-3).reverse();

    const statsMessage: MarketStatsMessage = {
      type: MessageType.MARKET_STATS,
      totalMarketCap,
      totalVolume24h,
      totalTrades24h,
      activeTokens: this.tokenData.size,
      topGainers,
      topLosers,
      timestamp: Date.now(),
      nonce: this.generateNonce()
    };

    await this.publish(subjects.marketStats, statsMessage);
    console.log(`📈 Market stats published: $${(totalMarketCap/1e9).toFixed(1)}B market cap`);
  }

  private async publish(subject: string, data: any): Promise<void> {
    try {
      if (!this.nats) {
        throw new Error('NATS connection not established');
      }
      await this.nats.publish(subject, sc.encode(JSON.stringify(data)));
      this.publishCount++;
    } catch (error) {
      console.error(`❌ Publish error to ${subject}:`, error);
    }
  }

  private startMetricsReporting(): void {
    setInterval(() => {
      const uptime = Math.floor((Date.now() - this.startTime) / 1000);
      const rate = (this.publishCount / uptime).toFixed(2);

      console.log(`
📊 Publisher Statistics:
   Messages Published: ${this.publishCount}
   Publish Rate: ${rate} msg/sec
   Active Tokens: ${this.tokenData.size}
   Uptime: ${uptime}s
      `);
    }, 30000); // Every 30 seconds
  }

  async shutdown(): Promise<void> {
    console.log('🛑 Shutting down publisher...');

    // Clear all intervals
    for (const interval of this.intervals.values()) {
      clearInterval(interval);
    }

    // Close NATS connection
    if (this.nats && !this.nats.isClosed()) {
      await this.nats.close();
    }

    console.log('✅ Publisher shutdown complete');
  }
}

// Start the publisher
const publisher = new OdinPublisher();

process.on('SIGINT', async () => {
  await publisher.shutdown();
  process.exit(0);
});

process.on('SIGTERM', async () => {
  await publisher.shutdown();
  process.exit(0);
});

// Initialize
publisher.initialize().catch(error => {
  console.error('❌ Failed to start publisher:', error);
  process.exit(1);
});