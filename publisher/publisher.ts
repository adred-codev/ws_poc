import express, { Request, Response } from 'express';
import { connect, NatsConnection, StringCodec } from 'nats';
import { config, subjects } from './config/odin.config.js';
import cors from 'cors';

const sc = StringCodec();

interface TokenData {
  id: string;
  price: number;
  volume24h: number;
  marketCap: number;
  priceChange24h: number;
  lastUpdate: number;
  trend: 'up' | 'down' | 'neutral';
  volatility: number;
}

interface PublisherStats {
  messagesPublished: number;
  startTime: number;
  errors: number;
}

interface ControlAPIRequest {
  action: 'start' | 'stop' | 'configure';
  connections?: number;
  messagesPerSecond?: number;
  duration?: number;
  tokens?: string[];
}

interface ControlAPIResponse {
  success: boolean;
  message: string;
  stats?: PublisherStats;
  activeTokens?: string[];
  configuration?: {
    connections: number;
    messagesPerSecond: number;
    tokens: string[];
  };
}

interface PriceUpdate {
  type: string;
  tokenId: string;
  price: number;
  volume24h: number;
  marketCap: number;
  priceChange24h: number;
  timestamp: number;
  source: string;
  nonce: string;
}

interface MarketStats {
  type: string;
  totalMarketCap: number;
  totalVolume24h: number;
  activeTokens: number;
  timestamp: number;
  topTokens: Array<{
    tokenId: string;
    price: number;
    marketCap: number;
    priceChange24h: number;
  }>;
  nonce: string;
}

class TokenPricePublisher {
  private nats: NatsConnection | null = null;
  private tokens: Map<string, TokenData> = new Map();
  private publishingIntervals: Map<string, NodeJS.Timeout> = new Map();
  private stats: PublisherStats = {
    messagesPublished: 0,
    startTime: Date.now(),
    errors: 0
  };
  private app!: express.Application;
  private httpServer: any;
  private isPublishing: boolean = false;
  private targetMessagesPerSecond: number = 0;
  private targetConnections: number = 0;

  async initialize(): Promise<void> {
    await this.setupNATS();
    this.initializeTokenData();
    this.setupHTTPServer();
    console.log('🚀 Token Price Publisher initialized with HTTP control API');
  }

  private async setupNATS(): Promise<void> {
    try {
      console.log('📡 Connecting to NATS...', config.nats.url);
      this.nats = await connect({
        servers: [config.nats.url],
        reconnect: true,
        maxReconnectAttempts: 10,
        reconnectTimeWait: 2000
      });
      console.log('✅ Connected to NATS server');

      // Initialize JetStream for guaranteed delivery
      const js = this.nats.jetstream();
      const jsm = await this.nats.jetstreamManager();

      // Ensure stream exists (server creates it, but publisher can verify)
      try {
        const streamInfo = await jsm.streams.info("ODIN_TOKENS");
        console.log(`✅ JetStream stream exists: ODIN_TOKENS (${streamInfo.state.messages} messages)`);
      } catch (err) {
        // Stream doesn't exist, server will create it
        console.log('ℹ️  JetStream stream will be created by server on first connection');
      }

      console.log('✅ JetStream initialized for reliable publishing');
    } catch (error) {
      console.error('❌ NATS connection failed:', error);
      process.exit(1);
    }
  }

  private initializeTokenData(): void {
    // Initialize with realistic starting prices and volumes
    const tokenDefaults: Record<string, { price: number; volume24h: number; marketCap: number }> = {
      'BTC': { price: 43250.00, volume24h: 125000000, marketCap: 850000000000 },
      'ETH': { price: 2580.50, volume24h: 85000000, marketCap: 310000000000 },
      'ODIN': { price: 0.125, volume24h: 2500000, marketCap: 125000000 },
      'SOL': { price: 98.75, volume24h: 15000000, marketCap: 42000000000 },
      'DOGE': { price: 0.078, volume24h: 8500000, marketCap: 11000000000 }
    };

    config.simulation.tokens.forEach(tokenId => {
      const defaults = tokenDefaults[tokenId] || {
        price: Math.random() * 100,
        volume24h: Math.random() * 1000000,
        marketCap: Math.random() * 1000000000
      };

      this.tokens.set(tokenId, {
        id: tokenId,
        price: defaults.price,
        volume24h: defaults.volume24h,
        marketCap: defaults.marketCap,
        priceChange24h: 0,
        lastUpdate: Date.now(),
        trend: 'neutral',
        volatility: Math.random() * 0.1 + 0.02 // 2-12% volatility
      });
    });

    console.log('💰 Initialized token data:', Array.from(this.tokens.keys()));
  }

  private setupHTTPServer(): void {
    this.app = express();
    this.app.use(cors());
    this.app.use(express.json());

    // Control endpoint for stress testing
    this.app.post('/control', (req: Request<{}, ControlAPIResponse, ControlAPIRequest>, res: Response<ControlAPIResponse>) => {
      this.handleControlRequest(req, res);
    });

    // Stats endpoint
    this.app.get('/stats', (req: Request, res: Response) => {
      res.json({
        success: true,
        stats: this.stats,
        isPublishing: this.isPublishing,
        activeTokens: Array.from(this.tokens.keys()),
        configuration: {
          connections: this.targetConnections,
          messagesPerSecond: this.targetMessagesPerSecond,
          tokens: Array.from(this.tokens.keys())
        }
      });
    });

    // Health check
    this.app.get('/health', (req: Request, res: Response) => {
      res.json({
        status: 'healthy',
        nats: this.nats?.isClosed() ? 'disconnected' : 'connected',
        publishing: this.isPublishing,
        uptime: Date.now() - this.stats.startTime
      });
    });

    const port = 3003;
    this.httpServer = this.app.listen(port, () => {
      console.log(`🌐 Publisher HTTP control API running on port ${port}`);
      console.log(`📊 Stats: http://localhost:${port}/stats`);
      console.log(`🎛️  Control: POST http://localhost:${port}/control`);
    });
  }

  private handleControlRequest(req: Request<{}, ControlAPIResponse, ControlAPIRequest>, res: Response<ControlAPIResponse>): void {
    const { action, connections, messagesPerSecond, duration, tokens } = req.body;

    try {
      switch (action) {
        case 'start':
          if (this.isPublishing) {
            res.json({ success: false, message: 'Publishing already active' });
            return;
          }

          // Configure parameters
          this.targetConnections = connections || 100;
          this.targetMessagesPerSecond = messagesPerSecond || 10;

          // Update token list if provided
          if (tokens && tokens.length > 0) {
            // Clear existing tokens and reinitialize with new list
            this.tokens.clear();
            tokens.forEach(tokenId => {
              this.tokens.set(tokenId, {
                id: tokenId,
                price: Math.random() * 100,
                volume24h: Math.random() * 1000000,
                marketCap: Math.random() * 1000000000,
                priceChange24h: 0,
                lastUpdate: Date.now(),
                trend: 'neutral',
                volatility: Math.random() * 0.1 + 0.02
              });
            });
          }

          this.startPublishing(messagesPerSecond);

          // Auto-stop after duration if specified
          if (duration && duration > 0) {
            setTimeout(() => {
              this.stopPublishing();
              console.log(`⏹️  Auto-stopped publishing after ${duration}ms`);
            }, duration);
          }

          res.json({
            success: true,
            message: `Started publishing with ${this.targetMessagesPerSecond} msg/sec for ${this.tokens.size} tokens`,
            stats: this.stats,
            activeTokens: Array.from(this.tokens.keys())
          });
          break;

        case 'stop':
          if (!this.isPublishing) {
            res.json({ success: false, message: 'Publishing not active' });
            return;
          }

          this.stopPublishing();
          res.json({
            success: true,
            message: 'Publishing stopped',
            stats: this.stats
          });
          break;

        case 'configure':
          this.targetConnections = connections || this.targetConnections;
          this.targetMessagesPerSecond = messagesPerSecond || this.targetMessagesPerSecond;

          if (tokens && tokens.length > 0) {
            // Update tokens without restarting
            this.tokens.clear();
            tokens.forEach(tokenId => {
              this.tokens.set(tokenId, {
                id: tokenId,
                price: Math.random() * 100,
                volume24h: Math.random() * 1000000,
                marketCap: Math.random() * 1000000000,
                priceChange24h: 0,
                lastUpdate: Date.now(),
                trend: 'neutral',
                volatility: Math.random() * 0.1 + 0.02
              });
            });
          }

          res.json({
            success: true,
            message: 'Configuration updated',
            configuration: {
              connections: this.targetConnections,
              messagesPerSecond: this.targetMessagesPerSecond,
              tokens: Array.from(this.tokens.keys())
            }
          });
          break;

        default:
          res.json({ success: false, message: 'Invalid action. Use start, stop, or configure' });
      }
    } catch (error) {
      console.error('❌ Control API error:', error);
      res.json({ success: false, message: 'Internal server error' });
    }
  }

  private startPublishing(messagesPerSecond?: number): void {
    if (this.isPublishing) {
      console.log('⚠️  Publishing already active');
      return;
    }

    this.isPublishing = true;
    const targetRate = messagesPerSecond || this.targetMessagesPerSecond || 10;
    const intervalMs = Math.max(100, 1000 / (targetRate / this.tokens.size)); // Min 100ms interval

    console.log(`▶️  Starting publishing at ${targetRate} msg/sec (${intervalMs}ms interval per token)`);

    this.tokens.forEach((_, tokenId) => {
      const interval = setInterval(() => {
        if (this.isPublishing) {
          this.publishPriceUpdate(tokenId);
        }
      }, intervalMs);

      this.publishingIntervals.set(tokenId, interval);
    });

    // Publish market statistics less frequently
    const statsInterval = setInterval(() => {
      if (this.isPublishing) {
        this.publishMarketStats();
      }
    }, 30000); // Every 30 seconds

    this.publishingIntervals.set('market-stats', statsInterval);
  }

  private stopPublishing(): void {
    if (!this.isPublishing) {
      return;
    }

    console.log('⏹️  Stopping publishing...');
    this.isPublishing = false;

    // Clear all intervals
    for (const interval of this.publishingIntervals.values()) {
      clearInterval(interval);
    }
    this.publishingIntervals.clear();

    console.log('✅ Publishing stopped');
  }

  private getUpdateInterval(tokenId: string): number {
    // Simulate different update frequencies
    const intervals: Record<string, number> = {
      'BTC': 1000,  // 1 second (most active)
      'ETH': 1500,  // 1.5 seconds
      'ODIN': 2000, // 2 seconds
      'SOL': 2500,  // 2.5 seconds
      'DOGE': 3000  // 3 seconds
    };
    return intervals[tokenId] || config.simulation.tradeFrequency;
  }

  private publishPriceUpdate(tokenId: string): void {
    const token = this.tokens.get(tokenId);
    if (!token || !this.nats) return;

    try {
      // Simulate price movement
      const priceChange = this.simulatePriceMovement(token);
      const oldPrice = token.price;
      token.price = Math.max(0.0001, oldPrice + priceChange); // Prevent negative prices

      // Calculate 24h change percentage
      const changePercent = ((token.price - oldPrice) / oldPrice) * 100;
      token.priceChange24h = (token.priceChange24h * 0.9) + (changePercent * 0.1); // Smooth the change

      // Update volume (simulate trading activity)
      token.volume24h *= (0.95 + Math.random() * 0.1); // ±5% volume change

      // Update market cap
      token.marketCap = token.price * 1000000; // Simplified market cap calculation

      // Update timestamp
      token.lastUpdate = Date.now();

      // Create price update message
      const priceUpdate: PriceUpdate = {
        type: 'price:update',
        tokenId: token.id,
        price: Number(token.price.toFixed(6)),
        volume24h: Math.floor(token.volume24h),
        marketCap: Math.floor(token.marketCap),
        priceChange24h: Number(token.priceChange24h.toFixed(2)),
        timestamp: token.lastUpdate,
        source: 'simulator',
        nonce: this.generateNonce()
      };

      // Publish to NATS using hierarchical subject format
      // Format: odin.token.{SYMBOL}.trade (e.g., odin.token.BTC.trade)
      const subject = subjects.tokenTrade(tokenId);
      this.nats.publish(subject, sc.encode(JSON.stringify(priceUpdate)));

      this.stats.messagesPublished++;

      // Log occasional updates
      if (this.stats.messagesPublished % 20 === 0) {
        console.log(`📊 Published update for ${tokenId}: $${priceUpdate.price} (${priceUpdate.priceChange24h.toFixed(2)}%)`);
      }

    } catch (error) {
      console.error(`❌ Error publishing ${tokenId} update:`, error);
      this.stats.errors++;
    }
  }

  private simulatePriceMovement(token: TokenData): number {
    const { volatility } = token;

    // Random walk with slight trend bias
    const randomChange = (Math.random() - 0.5) * token.price * volatility;

    // Add some trend persistence
    if (token.trend === 'up') {
      return randomChange + (Math.random() * token.price * 0.001);
    } else if (token.trend === 'down') {
      return randomChange - (Math.random() * token.price * 0.001);
    }

    // Randomly change trend
    if (Math.random() < 0.05) { // 5% chance to change trend
      token.trend = Math.random() < 0.4 ? 'up' : Math.random() < 0.8 ? 'down' : 'neutral';
    }

    return randomChange;
  }

  private publishMarketStats(): void {
    if (!this.nats) return;

    try {
      const totalMarketCap = Array.from(this.tokens.values())
        .reduce((sum, token) => sum + token.marketCap, 0);

      const totalVolume = Array.from(this.tokens.values())
        .reduce((sum, token) => sum + token.volume24h, 0);

      const marketStats: MarketStats = {
        type: 'market:statistics',
        totalMarketCap: Math.floor(totalMarketCap),
        totalVolume24h: Math.floor(totalVolume),
        activeTokens: this.tokens.size,
        timestamp: Date.now(),
        topTokens: this.getTopTokens(3),
        nonce: this.generateNonce()
      };

      this.nats.publish(subjects.marketStats, sc.encode(JSON.stringify(marketStats)));
      console.log(`📈 Published market stats: $${(totalMarketCap / 1e9).toFixed(1)}B market cap`);

    } catch (error) {
      console.error('❌ Error publishing market stats:', error);
      this.stats.errors++;
    }
  }

  private getTopTokens(count: number): Array<{
    tokenId: string;
    price: number;
    marketCap: number;
    priceChange24h: number;
  }> {
    return Array.from(this.tokens.values())
      .sort((a, b) => b.marketCap - a.marketCap)
      .slice(0, count)
      .map(token => ({
        tokenId: token.id,
        price: Number(token.price.toFixed(6)),
        marketCap: Math.floor(token.marketCap),
        priceChange24h: Number(token.priceChange24h.toFixed(2))
      }));
  }

  private generateNonce(): string {
    return `${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
  }

  printStats(): void {
    const uptime = Date.now() - this.stats.startTime;
    const rate = this.stats.messagesPublished / (uptime / 1000);

    console.log('\n📊 Publisher Statistics:');
    console.log(`   Messages Published: ${this.stats.messagesPublished}`);
    console.log(`   Publish Rate: ${rate.toFixed(1)} msg/sec`);
    console.log(`   Errors: ${this.stats.errors}`);
    console.log(`   Uptime: ${Math.floor(uptime / 1000)}s`);
    console.log(`   Active Tokens: ${this.tokens.size}`);
  }

  async shutdown(): Promise<void> {
    console.log('🛑 Shutting down publisher...');

    // Stop publishing
    this.stopPublishing();

    // Close HTTP server
    if (this.httpServer) {
      this.httpServer.close();
    }

    // Close NATS connection
    if (this.nats && !this.nats.isClosed()) {
      await this.nats.close();
    }

    console.log('✅ Publisher shutdown complete');
  }
}

// Start publisher
const publisher = new TokenPricePublisher();

// Graceful shutdown
process.on('SIGINT', async () => {
  publisher.printStats();
  await publisher.shutdown();
  process.exit(0);
});

process.on('SIGTERM', async () => {
  await publisher.shutdown();
  process.exit(0);
});

// Print stats every 30 seconds
setInterval(() => {
  publisher.printStats();
}, 30000);

// Initialize publisher
publisher.initialize().catch((error) => {
  console.error('❌ Failed to start publisher:', error);
  process.exit(1);
});