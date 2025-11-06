import * as dotenv from 'dotenv';
import { RedpandaPublisher } from './redpanda-publisher.js';
import { ApiServer } from './api-server.js';

// Load environment variables
dotenv.config();

const KAFKA_BROKERS = process.env.KAFKA_BROKERS?.split(',') || [
  'localhost:19092',
];
const API_PORT = parseInt(process.env.API_PORT || '3001', 10);

async function main() {
  console.log('[Publisher] Starting Odin Publisher v2.0.0');
  console.log(`[Publisher] Kafka brokers: ${KAFKA_BROKERS.join(', ')}`);
  console.log(`[Publisher] API port: ${API_PORT}`);

  // Initialize publisher
  const publisher = new RedpandaPublisher(KAFKA_BROKERS);

  try {
    // Connect to Redpanda
    await publisher.connect();
    console.log('[Publisher] Successfully connected to Redpanda');

    // Start API server
    new ApiServer(publisher, API_PORT);
    console.log('[Publisher] API server started');
    console.log('[Publisher] Ready to accept requests');
    console.log('[Publisher] ');
    console.log('[Publisher] API Endpoints:');
    console.log(
      `[Publisher]   GET  http://localhost:${API_PORT}/health   - Health check`
    );
    console.log(
      `[Publisher]   GET  http://localhost:${API_PORT}/status   - Current status`
    );
    console.log(
      `[Publisher]   POST http://localhost:${API_PORT}/start    - Start event generation`
    );
    console.log(
      `[Publisher]   POST http://localhost:${API_PORT}/stop     - Stop event generation`
    );
    console.log(
      `[Publisher]   POST http://localhost:${API_PORT}/rate     - Update event rate`
    );
    console.log('[Publisher] ');
    console.log(
      '[Publisher] Example: Start publishing 100 events/sec for 3 tokens:'
    );
    console.log(
      `[Publisher]   curl -X POST http://localhost:${API_PORT}/start -H "Content-Type: application/json" -d '{"rate": 100, "tokenIds": ["token1", "token2", "token3"]}'`
    );
  } catch (error) {
    console.error('[Publisher] Failed to start:', error);
    process.exit(1);
  }

  // Graceful shutdown
  const shutdown = async () => {
    console.log('[Publisher] Shutting down...');
    await publisher.disconnect();
    console.log('[Publisher] Disconnected from Redpanda');
    process.exit(0);
  };

  process.on('SIGINT', shutdown);
  process.on('SIGTERM', shutdown);
}

main().catch(error => {
  console.error('[Publisher] Fatal error:', error);
  process.exit(1);
});
