import { connect, StringCodec } from 'nats';
import { config, subjects } from './config.js';

const sc = StringCodec();

class TokenPricePublisher {
  constructor() {
    this.nats = null;
    this.tokens = new Map();
    this.publishingIntervals = new Map();
    this.stats = {
      messagesPublished: 0,
      startTime: Date.now(),
      errors: 0
    };
  }

  async initialize() {
    await this.setupNATS();
    this.initializeTokenData();
    this.startPublishing();
    console.log('🚀 Token Price Publisher initialized');
  }

  async setupNATS() {
    try {
      console.log('📡 Connecting to NATS...', config.nats.url);
      this.nats = await connect({
        servers: [config.nats.url],
        reconnect: true,
        maxReconnectAttempts: 10,
        reconnectTimeWait: 2000
      });
      console.log('✅ Connected to NATS server');
    } catch (error) {
      console.error('❌ NATS connection failed:', error);
      process.exit(1);
    }
  }

  initializeTokenData() {
    // Initialize with realistic starting prices and volumes
    const tokenDefaults = {
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
        trend: 'neutral', // up, down, neutral
        volatility: Math.random() * 0.1 + 0.02 // 2-12% volatility
      });
    });

    console.log('💰 Initialized token data:', Array.from(this.tokens.keys()));
  }

  startPublishing() {
    config.simulation.tokens.forEach(tokenId => {
      // Different update frequencies for different tokens (simulating real market)
      const updateInterval = this.getUpdateInterval(tokenId);

      const interval = setInterval(() => {
        this.publishPriceUpdate(tokenId);
      }, updateInterval);

      this.publishingIntervals.set(tokenId, interval);
      console.log(`⏰ Started publishing ${tokenId} every ${updateInterval}ms`);
    });

    // Publish market statistics less frequently
    const statsInterval = setInterval(() => {
      this.publishMarketStats();
    }, 30000); // Every 30 seconds

    this.publishingIntervals.set('market-stats', statsInterval);
  }

  getUpdateInterval(tokenId) {
    // Simulate different update frequencies
    const intervals = {
      'BTC': 1000,  // 1 second (most active)
      'ETH': 1500,  // 1.5 seconds
      'ODIN': 2000, // 2 seconds
      'SOL': 2500,  // 2.5 seconds
      'DOGE': 3000  // 3 seconds
    };
    return intervals[tokenId] || config.simulation.priceUpdateInterval;
  }

  publishPriceUpdate(tokenId) {
    const token = this.tokens.get(tokenId);
    if (!token) return;

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
      const priceUpdate = {
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

      // Publish to NATS
      const subject = subjects.tokenPrice(tokenId);
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

  simulatePriceMovement(token) {
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

  publishMarketStats() {
    try {
      const totalMarketCap = Array.from(this.tokens.values())
        .reduce((sum, token) => sum + token.marketCap, 0);

      const totalVolume = Array.from(this.tokens.values())
        .reduce((sum, token) => sum + token.volume24h, 0);

      const marketStats = {
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

  getTopTokens(count) {
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

  generateNonce() {
    return `${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
  }

  printStats() {
    const uptime = Date.now() - this.stats.startTime;
    const rate = this.stats.messagesPublished / (uptime / 1000);

    console.log('\n📊 Publisher Statistics:');
    console.log(`   Messages Published: ${this.stats.messagesPublished}`);
    console.log(`   Publish Rate: ${rate.toFixed(1)} msg/sec`);
    console.log(`   Errors: ${this.stats.errors}`);
    console.log(`   Uptime: ${Math.floor(uptime / 1000)}s`);
    console.log(`   Active Tokens: ${this.tokens.size}`);
  }

  async shutdown() {
    console.log('🛑 Shutting down publisher...');

    // Clear all intervals
    for (const interval of this.publishingIntervals.values()) {
      clearInterval(interval);
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