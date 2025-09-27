import express from 'express';
import { WebSocketServer } from 'ws';
import { createServer } from 'http';
import cors from 'cors';
import jwt from 'jsonwebtoken';

// Application Services
import { ConnectionService } from './application/services/ConnectionService';
import { MessageService } from './application/services/MessageService';

// Infrastructure
import { InMemoryClientRepository } from './infrastructure/persistence/InMemoryClientRepository';
import { WebSocketMessageRepository } from './infrastructure/websocket/WebSocketMessageRepository';
import { NatsEventSubscriber } from './infrastructure/nats/NatsEventSubscriber';

// Presentation
import { WebSocketController } from './presentation/controllers/WebSocketController';
import { HealthController } from './presentation/controllers/HealthController';

// Configuration
import { config } from './config/odin.config';

/**
 * Clean Architecture WebSocket Server
 *
 * Follows clean architecture principles:
 * - Domain layer: Pure business logic, no dependencies
 * - Application layer: Use cases and services
 * - Infrastructure layer: External concerns (NATS, WebSocket, persistence)
 * - Presentation layer: Controllers and HTTP routes
 */
export class CleanArchitectureServer {
  // Infrastructure
  private readonly app = express();
  private readonly server = createServer(this.app);
  private wss: WebSocketServer | null = null;

  // Repositories (Infrastructure)
  private readonly clientRepository = new InMemoryClientRepository();
  private webSocketRepository!: WebSocketMessageRepository;
  private natsSubscriber!: NatsEventSubscriber;

  // Application Services
  private connectionService!: ConnectionService;
  private messageService!: MessageService;

  // Presentation Controllers
  private webSocketController!: WebSocketController;
  private healthController!: HealthController;

  async initialize(): Promise<void> {
    console.log('🏗️ Initializing Clean Architecture Server...');

    // 1. Dependency Injection - Wire up the layers
    this.setupDependencyInjection();

    // 2. Setup infrastructure
    await this.setupInfrastructure();

    // 3. Setup presentation layer
    this.setupPresentationLayer();

    console.log('🚀 Clean Architecture Server initialized successfully');
  }

  private setupDependencyInjection(): void {
    console.log('🔗 Setting up dependency injection...');

    // Infrastructure layer
    this.webSocketRepository = new WebSocketMessageRepository(this.clientRepository);
    this.natsSubscriber = new NatsEventSubscriber(
      {
        url: config.nats.url,
        reconnect: true,
        maxReconnectAttempts: 10
      },
      this.messageService // Will be set in circular dependency resolution
    );

    // Application layer
    this.connectionService = new ConnectionService(
      this.clientRepository,
      this.webSocketRepository
    );

    this.messageService = new MessageService(
      this.webSocketRepository,
      this.clientRepository
    );

    // Resolve circular dependency
    this.natsSubscriber = new NatsEventSubscriber(
      {
        url: config.nats.url,
        reconnect: true,
        maxReconnectAttempts: 10
      },
      this.messageService
    );

    // Presentation layer
    this.webSocketController = new WebSocketController(
      this.connectionService,
      this.messageService,
      this.webSocketRepository
    );

    this.healthController = new HealthController(
      this.connectionService,
      this.messageService,
      this.natsSubscriber
    );

    console.log('✅ Dependency injection complete');
  }

  private async setupInfrastructure(): Promise<void> {
    console.log('🔧 Setting up infrastructure...');

    // Connect to NATS
    await this.natsSubscriber.connect();
    await this.natsSubscriber.subscribeToOdinEvents();

    // Setup HTTP server
    this.setupHTTPServer();

    // Setup WebSocket server
    this.setupWebSocketServer();

    // Start background tasks
    this.startBackgroundTasks();

    console.log('✅ Infrastructure setup complete');
  }

  private setupHTTPServer(): void {
    this.app.use(cors());
    this.app.use(express.json());

    // Health endpoints
    this.app.get('/health', this.healthController.getHealth.bind(this.healthController));
    this.app.get('/stats', this.healthController.getStats.bind(this.healthController));
    this.app.get('/metrics', this.healthController.getMetrics.bind(this.healthController));
    this.app.post('/metrics/reset', this.healthController.resetMetrics.bind(this.healthController));

    // Development JWT endpoint
    if (config.env === 'development') {
      this.app.post('/auth/token', (req, res) => {
        const { userId = 'test-user' } = req.body;
        const token = jwt.sign(
          {
            userId,
            iat: Math.floor(Date.now() / 1000),
            exp: Math.floor(Date.now() / 1000) + config.jwt.expiry
          },
          config.jwt.secret
        );
        res.json({ token, userId });
      });
    }

    this.server.listen(config.server.httpPort, () => {
      console.log(`🌐 HTTP server running on port ${config.server.httpPort}`);
    });
  }

  private setupWebSocketServer(): void {
    this.wss = new WebSocketServer({
      server: this.server,
      path: '/ws'
    });

    this.wss.on('connection', (ws, request) => {
      this.webSocketController.handleConnection(ws, request);
    });

    console.log(`🔌 WebSocket server ready on port ${config.server.httpPort}/ws`);
  }

  private setupPresentationLayer(): void {
    console.log('🎯 Setting up presentation layer...');
    // Presentation layer is already set up through dependency injection
    console.log('✅ Presentation layer ready');
  }

  private startBackgroundTasks(): void {
    console.log('⏰ Starting background tasks...');

    // Cleanup inactive clients every minute
    setInterval(async () => {
      try {
        const cleanedUp = await this.connectionService.cleanupInactiveClients(60000);
        if (cleanedUp > 0) {
          console.log(`🧹 Cleaned up ${cleanedUp} inactive clients`);
        }
      } catch (error) {
        console.error('❌ Error cleaning up clients:', error);
      }
    }, 60000);

    // Send heartbeat every 30 seconds
    setInterval(async () => {
      try {
        const heartbeatsSent = await this.messageService.sendHeartbeat();
        if (heartbeatsSent > 0) {
          console.log(`💓 Sent heartbeat to ${heartbeatsSent} clients`);
        }
      } catch (error) {
        console.error('❌ Error sending heartbeat:', error);
      }
    }, 30000);

    // Report metrics every 30 seconds
    setInterval(async () => {
      try {
        const connectionMetrics = await this.connectionService.getConnectionMetrics();
        const messageMetrics = this.messageService.getMetrics();

        console.log(`
📊 Clean Architecture Server Metrics:
   Connections: ${connectionMetrics.currentConnections}
   Messages Delivered: ${messageMetrics.messagesDelivered}
   Duplicates Dropped: ${messageMetrics.duplicatesDropped}
   Average Latency: ${messageMetrics.averageLatency.toFixed(2)}ms
   Peak Latency: ${messageMetrics.peakLatency}ms
   Error Rate: ${(messageMetrics.errorRate * 100).toFixed(2)}%
        `);
      } catch (error) {
        console.error('❌ Error reporting metrics:', error);
      }
    }, 30000);

    console.log('✅ Background tasks started');
  }

  async shutdown(): Promise<void> {
    console.log('🛑 Shutting down Clean Architecture Server...');

    // Close WebSocket server
    if (this.wss) {
      this.wss.close();
    }

    // Disconnect all clients
    const allClients = await this.connectionService.getAllClients();
    for (const client of allClients) {
      await this.connectionService.disconnectClient(client.id);
    }

    // Disconnect NATS
    await this.natsSubscriber.disconnect();

    // Close HTTP server
    this.server.close();

    console.log('✅ Clean Architecture Server shutdown complete');
  }
}

// Application Entry Point
const server = new CleanArchitectureServer();

// Graceful shutdown
process.on('SIGINT', async () => {
  await server.shutdown();
  process.exit(0);
});

process.on('SIGTERM', async () => {
  await server.shutdown();
  process.exit(0);
});

// Initialize server
server.initialize().catch((error) => {
  console.error('❌ Failed to start Clean Architecture Server:', error);
  process.exit(1);
});