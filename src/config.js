import dotenv from 'dotenv';
dotenv.config();

export const config = {
  nats: {
    url: process.env.NATS_URL || 'nats://localhost:4222',
    stream: process.env.NATS_STREAM || 'odin-tokens'
  },
  server: {
    wsPort: parseInt(process.env.WS_PORT) || 8080,
    httpPort: parseInt(process.env.HTTP_PORT) || 3001
  },
  jwt: {
    secret: process.env.JWT_SECRET || 'dev-secret-change-in-production'
  },
  simulation: {
    priceUpdateInterval: parseInt(process.env.PRICE_UPDATE_INTERVAL) || 2000,
    tokens: (process.env.TOKENS || 'BTC,ETH,ODIN,SOL,DOGE').split(',')
  },
  env: process.env.NODE_ENV || 'development',
  logLevel: process.env.LOG_LEVEL || 'info'
};

// NATS subject patterns
export const subjects = {
  tokenPrice: (tokenId) => `odin.tokens.${tokenId}.price`,
  tokenVolume: (tokenId) => `odin.tokens.${tokenId}.volume`,
  batchUpdate: 'odin.tokens.batch.update',
  marketStats: 'odin.market.statistics',
  systemHealth: 'odin.system.health'
};

console.log('🔧 Configuration loaded:', {
  natsUrl: config.nats.url,
  wsPort: config.server.wsPort,
  httpPort: config.server.httpPort,
  tokens: config.simulation.tokens,
  env: config.env
});