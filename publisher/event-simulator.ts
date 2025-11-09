import { TokenEvent, EventType } from './types/event-types.js';

export class EventSimulator {
  private tokenIds: string[];
  private isRunning: boolean = false;
  private intervalId?: NodeJS.Timeout;
  private statsIntervalId?: NodeJS.Timeout;
  private eventCount: number = 0;
  private lastLogTime: number = 0;
  private lastEventCount: number = 0;

  constructor(tokenIds: string[]) {
    this.tokenIds = tokenIds;
  }

  /**
   * Start generating events at specified rate
   */
  start(
    eventsPerSecond: number,
    onEvent: (event: TokenEvent) => void | Promise<void>
  ): void {
    if (this.isRunning) {
      console.warn('[EventSimulator] Already running');
      return;
    }

    this.isRunning = true;
    const intervalMs = 1000 / eventsPerSecond;

    // Reset counters
    this.eventCount = 0;
    this.lastLogTime = Date.now();
    this.lastEventCount = 0;

    console.log(`[EventSimulator] Starting at ${eventsPerSecond} events/sec`);

    // Start event generation
    this.intervalId = setInterval(() => {
      const event = this.generateRandomEvent();
      this.eventCount++;
      console.log(
        `[EventSimulator] Generated ${event.type} for token ${event.tokenId}`
      );
      void onEvent(event);
    }, intervalMs);

    // Start periodic stats logging (every 10 seconds)
    this.statsIntervalId = setInterval(() => {
      const now = Date.now();
      const elapsedSeconds = (now - this.lastLogTime) / 1000;
      const eventsGenerated = this.eventCount - this.lastEventCount;
      const actualRate = eventsGenerated / elapsedSeconds;

      console.log(
        `[EventSimulator] Generated ${eventsGenerated} events in last ${elapsedSeconds.toFixed(1)}s (${actualRate.toFixed(1)} events/sec) | Total: ${this.eventCount}`
      );

      this.lastLogTime = now;
      this.lastEventCount = this.eventCount;
    }, 10000); // Log every 10 seconds
  }

  /**
   * Stop generating events
   */
  stop(): void {
    if (!this.isRunning) {
      return;
    }

    if (this.intervalId) {
      clearInterval(this.intervalId);
      this.intervalId = undefined;
    }

    if (this.statsIntervalId) {
      clearInterval(this.statsIntervalId);
      this.statsIntervalId = undefined;
    }

    this.isRunning = false;
    console.log(
      `[EventSimulator] Stopped after generating ${this.eventCount} total events`
    );
  }

  /**
   * Generate a random event
   */
  private generateRandomEvent(): TokenEvent {
    const tokenId = this.randomElement(this.tokenIds);
    const eventType = this.randomEventType();
    const timestamp = Date.now();

    return {
      type: eventType,
      tokenId,
      timestamp,
      data: this.generateEventData(eventType, tokenId),
    };
  }

  /**
   * Generate appropriate data for each event type
   */
  private generateEventData(
    eventType: EventType,
    tokenId: string
  ): Record<string, any> {
    switch (eventType) {
      case 'TRADE_EXECUTED':
      case 'BUY_COMPLETED':
      case 'SELL_COMPLETED':
        return {
          price: this.randomPrice(),
          amount: this.randomAmount(),
          buyer: this.randomAddress(),
          seller: this.randomAddress(),
          txHash: this.randomHash(),
        };

      case 'LIQUIDITY_ADDED':
      case 'LIQUIDITY_REMOVED':
        return {
          amount: this.randomAmount(),
          provider: this.randomAddress(),
          totalLiquidity: this.randomAmount() * 10,
        };

      case 'LIQUIDITY_REBALANCED':
        return {
          oldRatio: Math.random(),
          newRatio: Math.random(),
          totalLiquidity: this.randomAmount() * 10,
        };

      case 'METADATA_UPDATED':
      case 'TOKEN_NAME_CHANGED':
        return {
          name: `Token ${tokenId}`,
          symbol: this.randomSymbol(),
          description: 'Updated token metadata',
        };

      case 'TOKEN_FLAGS_CHANGED':
        return {
          flags: {
            verified: Math.random() > 0.5,
            featured: Math.random() > 0.8,
            trending: Math.random() > 0.7,
          },
        };

      case 'TWITTER_VERIFIED':
        return {
          twitterHandle: `@token${tokenId}`,
          verified: true,
          verifiedAt: Date.now(),
        };

      case 'SOCIAL_LINKS_UPDATED':
        return {
          twitter: `https://twitter.com/token${tokenId}`,
          telegram: `https://t.me/token${tokenId}`,
          website: `https://token${tokenId}.com`,
        };

      case 'COMMENT_POSTED':
        return {
          commentId: this.randomHash(),
          author: this.randomAddress(),
          content: 'This is a test comment',
          timestamp: Date.now(),
        };

      case 'COMMENT_PINNED':
        return {
          commentId: this.randomHash(),
          pinnedBy: this.randomAddress(),
        };

      case 'COMMENT_UPVOTED':
        return {
          commentId: this.randomHash(),
          voter: this.randomAddress(),
          totalUpvotes: Math.floor(Math.random() * 100),
        };

      case 'FAVORITE_TOGGLED':
        return {
          userId: this.randomAddress(),
          isFavorite: Math.random() > 0.5,
        };

      case 'TOKEN_CREATED':
        return {
          creator: this.randomAddress(),
          name: `Token ${tokenId}`,
          symbol: this.randomSymbol(),
          supply: this.randomAmount() * 1000000,
        };

      case 'TOKEN_LISTED':
        return {
          exchange: 'Odin DEX',
          initialPrice: this.randomPrice(),
          timestamp: Date.now(),
        };

      case 'PRICE_DELTA_UPDATED':
        return {
          price: this.randomPrice(),
          delta1h: (Math.random() - 0.5) * 20,
          delta24h: (Math.random() - 0.5) * 50,
          delta7d: (Math.random() - 0.5) * 100,
        };

      case 'HOLDER_COUNT_UPDATED':
        return {
          holderCount: Math.floor(Math.random() * 10000),
          change: Math.floor((Math.random() - 0.5) * 100),
        };

      case 'ANALYTICS_RECALCULATED':
        return {
          volume24h: this.randomAmount() * 10000,
          marketCap: this.randomAmount() * 1000000,
          liquidity: this.randomAmount() * 100000,
        };

      case 'TRENDING_UPDATED':
        return {
          rank: Math.floor(Math.random() * 100) + 1,
          score: Math.random() * 1000,
        };

      case 'BALANCE_UPDATED':
        return {
          user: this.randomAddress(),
          balance: this.randomAmount() * 1000,
          change: (Math.random() - 0.5) * 100,
        };

      case 'TRANSFER_COMPLETED':
        return {
          from: this.randomAddress(),
          to: this.randomAddress(),
          amount: this.randomAmount(),
          txHash: this.randomHash(),
        };

      default:
        return {};
    }
  }

  /**
   * Random event type with weighted distribution
   */
  private randomEventType(): EventType {
    const weights = {
      // High frequency events (50%)
      TRADE_EXECUTED: 20,
      BUY_COMPLETED: 15,
      SELL_COMPLETED: 15,

      // Medium frequency events (30%)
      LIQUIDITY_ADDED: 8,
      LIQUIDITY_REMOVED: 8,
      PRICE_DELTA_UPDATED: 7,
      ANALYTICS_RECALCULATED: 7,

      // Low frequency events (20%)
      METADATA_UPDATED: 5,
      COMMENT_POSTED: 5,
      BALANCE_UPDATED: 3,
      HOLDER_COUNT_UPDATED: 2,
      FAVORITE_TOGGLED: 2,
      COMMENT_UPVOTED: 1,
      TRENDING_UPDATED: 1,
      SOCIAL_LINKS_UPDATED: 1,

      // Rare events (<1%)
      TOKEN_CREATED: 0.5,
      TOKEN_LISTED: 0.5,
      TOKEN_NAME_CHANGED: 0.3,
      TOKEN_FLAGS_CHANGED: 0.3,
      TWITTER_VERIFIED: 0.2,
      COMMENT_PINNED: 0.2,
      LIQUIDITY_REBALANCED: 0.1,
      TRANSFER_COMPLETED: 0.1,
    };

    const totalWeight = Object.values(weights).reduce((sum, w) => sum + w, 0);
    let random = Math.random() * totalWeight;

    for (const [eventType, weight] of Object.entries(weights)) {
      random -= weight;
      if (random <= 0) {
        return eventType as EventType;
      }
    }

    return 'TRADE_EXECUTED';
  }

  private randomElement<T>(array: T[]): T {
    return array[Math.floor(Math.random() * array.length)];
  }

  private randomPrice(): number {
    return Math.random() * 100;
  }

  private randomAmount(): number {
    return Math.random() * 1000;
  }

  private randomAddress(): string {
    return '0x' + Math.random().toString(16).substring(2, 42);
  }

  private randomHash(): string {
    return '0x' + Math.random().toString(16).substring(2, 66);
  }

  private randomSymbol(): string {
    const letters = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ';
    const length = 3 + Math.floor(Math.random() * 3);
    return Array.from(
      { length },
      () => letters[Math.floor(Math.random() * letters.length)]
    ).join('');
  }
}
