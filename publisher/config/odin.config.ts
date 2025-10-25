import dotenv from 'dotenv';
import {
  MessageType,
  UpdateFrequencies,
  NatsSubjects,
  OdinConfig,
} from '../types/odin.types.js';

dotenv.config();

// Update frequencies (in ms) matching Odin requirements
export const updateFrequencies: UpdateFrequencies = {
  TOKEN_PRICES: 60 * 1000, // 1 minute
  PRICE_DELTAS: 60 * 1000, // 1 minute
  BTC_PRICE: 5 * 60 * 1000, // 5 minutes
  TOKEN_VOLUMES: 10 * 60 * 1000, // 10 minutes
  HOLDER_COUNTS: 10 * 60 * 1000, // 10 minutes
  STATISTICS: 60 * 60 * 1000, // 60 minutes
  USER_TRADES: 0, // Instant
};

// NATS subject hierarchy - Coarse-Grained Channel Format
// Format: odin.{channelType}.{identifier}
// Event types are included in message payload, not NATS subject
//
// Channel Types:
//   token - Token-related events (trade, liquidity, metadata, etc.)
//   user  - User-specific events (favorites, balances)
//   global - System-wide events (market stats, health)
//
// Architecture Decision:
// - Publisher publishes to coarse-grained subjects (e.g., odin.token.BTC)
// - Event type is in message payload (e.g., {type: 'token:trade', ...})
// - WebSocket server broadcasts to matching channels (e.g., token.BTC)
// - Clients receive ALL events for subscribed tokens and filter by type if needed
export const subjects: NatsSubjects = {
  // Coarse-grained token channel (all events for a token)
  token: (tokenId: string) => `odin.token.${tokenId}`,

  // User-specific channel (all events for a user)
  user: (userId: string) => `odin.user.${userId}`,

  // Global channel (system-wide events: market stats, health, etc.)
  global: 'odin.global',
};

// Performance metrics configuration
export const metricsConfig: MetricsConfig = {
  // Track these metrics
  metrics: [
    'messagesPublished',
    'messagesDelivered',
    'connectionCount',
    'averageLatency',
    'peakLatency',
    'errorRate',
    'reconnectionCount',
    'duplicatesDropped',
    'apiRequestsSaved',
    'bandwidthSaved',
  ],

  // Report intervals
  reportInterval: 30 * 1000, // 30 seconds

  // Alert thresholds
  thresholds: {
    latency: 50, // Alert if latency > 50ms
    errorRate: 0.01, // Alert if error rate > 1%
    connections: 100000, // Alert if connections > 100k
  },
};

// Scaling configuration
export const scalingConfig: ScalingConfig = {
  maxConnectionsPerInstance: 5000,
  autoScaleThreshold: 0.8, // Scale at 80% capacity
  cooldownPeriod: 300, // 5 minutes between scaling events
  minInstances: 2,
  maxInstances: 20,
};

// Migration configuration (dual-mode support)
export const migrationConfig: MigrationConfig = {
  enabled: false, // Disable migration for stress testing
  pollingEndpoint: '/api/poll',
  websocketEndpoint: '/ws',
  rolloutPercentage: 100, // Allow all WebSocket connections
  fallbackEnabled: true,
  dualModeDuration: 30 * 24 * 60 * 60 * 1000, // 30 days
};

export const config: OdinConfig = {
  nats: {
    url: process.env.NATS_URL || 'nats://localhost:4222',
    cluster: process.env.NATS_CLUSTER || 'odin-cluster',
    jetstream: {
      enabled: true,
      stream: 'ODIN_EVENTS',
      retention: 'limits',
      maxAge: 24 * 60 * 60 * 1000 * 7, // 7 days
      maxMsgs: 10000000,
      dedupWindow: 60 * 1000, // 1 minute dedup window
    },
  },
  server: {
    wsPort: parseInt(process.env.WS_PORT as string) || 8080,
    httpPort: parseInt(process.env.HTTP_PORT as string) || 3001,
  },
  redis: {
    enabled: false, // Disabled for now, using in-memory
    url: process.env.REDIS_URL || 'redis://localhost:6379',
    ttl: 60 * 60, // 1 hour TTL for connection state
  },
  jwt: {
    secret: process.env.JWT_SECRET || 'dev-secret-change-in-production',
    expiry: 24 * 60 * 60, // 24 hours
  },
  simulation: {
    tokens: (
      process.env.TOKENS || 'BTC,ETH,ODIN,SOL,DOGE,USDT,BNB,XRP,ADA,MATIC'
    ).split(','),
    tradeFrequency: 5000, // Simulate trade every 5 seconds
    enableScheduler: true,
    enableTradeSimulator: true,
  },
  env: process.env.NODE_ENV || 'development',
  logLevel: process.env.LOG_LEVEL || 'info',
};

console.log('🔧 Odin Configuration loaded:', {
  natsUrl: config.nats.url,
  jetstream: config.nats.jetstream.enabled,
  redis: config.redis.enabled,
  wsPort: config.server.wsPort,
  httpPort: config.server.httpPort,
  tokens: config.simulation.tokens.length,
  migration: migrationConfig.enabled,
  env: config.env,
});

// Additional type definitions for metrics and migration config
export interface MetricsConfig {
  metrics: string[];
  reportInterval: number;
  thresholds: {
    latency: number;
    errorRate: number;
    connections: number;
  };
}

export interface ScalingConfig {
  maxConnectionsPerInstance: number;
  autoScaleThreshold: number;
  cooldownPeriod: number;
  minInstances: number;
  maxInstances: number;
}

export interface MigrationConfig {
  enabled: boolean;
  pollingEndpoint: string;
  websocketEndpoint: string;
  rolloutPercentage: number;
  fallbackEnabled: boolean;
  dualModeDuration: number;
}
