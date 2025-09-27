import express from 'express';
import { WebSocketServer } from 'ws';
import { connect, StringCodec } from 'nats';
import jwt from 'jsonwebtoken';
import { config, subjects } from './config.js';
import { createServer } from 'http';
import cors from 'cors';

const sc = StringCodec();

class OdinWebSocketServer {
  constructor() {
    this.app = express();
    this.server = createServer(this.app);
    this.wss = null;
    this.nats = null;
    this.clients = new Map();
    this.stats = {
      totalConnections: 0,
      currentConnections: 0,
      messagesSent: 0,
      messagesReceived: 0,
      startTime: Date.now()
    };
  }

  async initialize() {
    await this.setupNATS();
    this.setupHTTPServer();
    this.setupWebSocketServer();
    this.startHealthCheck();
    console.log('🚀 Odin WebSocket Server initialized');
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

      // Subscribe to all token price updates
      const subscription = this.nats.subscribe('odin.tokens.*.price');
      console.log('📬 Subscribed to:', 'odin.tokens.*.price');

      // Handle incoming NATS messages (non-blocking)
      (async () => {
        for await (const msg of subscription) {
          try {
            const data = JSON.parse(sc.decode(msg.data));
            this.broadcastToClients(data);
            this.stats.messagesReceived++;
          } catch (error) {
            console.error('❌ Error processing NATS message:', error);
          }
        }
      })();

    } catch (error) {
      console.error('❌ NATS connection failed:', error);
      process.exit(1);
    }
  }

  setupHTTPServer() {
    this.app.use(cors());
    this.app.use(express.json());

    // Health check endpoint
    this.app.get('/health', (req, res) => {
      const uptime = Date.now() - this.stats.startTime;
      res.json({
        status: 'healthy',
        uptime: Math.floor(uptime / 1000),
        nats: this.nats?.isClosed() ? 'disconnected' : 'connected',
        websocket: {
          currentConnections: this.stats.currentConnections,
          totalConnections: this.stats.totalConnections
        },
        stats: this.stats
      });
    });

    // Stats endpoint
    this.app.get('/stats', (req, res) => {
      res.json({
        ...this.stats,
        uptime: Math.floor((Date.now() - this.stats.startTime) / 1000),
        clients: Array.from(this.clients.keys()).length
      });
    });

    // Generate test JWT token endpoint (development only)
    if (config.env === 'development') {
      this.app.post('/auth/token', (req, res) => {
        const { userId = 'test-user' } = req.body;
        const token = jwt.sign(
          {
            userId,
            iat: Math.floor(Date.now() / 1000),
            exp: Math.floor(Date.now() / 1000) + (24 * 60 * 60) // 24 hours
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

  setupWebSocketServer() {
    this.wss = new WebSocketServer({
      server: this.server,
      path: '/ws'
    });

    this.wss.on('connection', (ws, request) => {
      this.handleNewConnection(ws, request);
    });

    console.log(`🔌 WebSocket server ready on port ${config.server.httpPort}/ws`);
  }

  handleNewConnection(ws, request) {
    const clientId = this.generateClientId();
    const clientInfo = {
      id: clientId,
      connectedAt: Date.now(),
      ip: request.socket.remoteAddress,
      userAgent: request.headers['user-agent']
    };

    console.log(`🔗 New client connected: ${clientId}`);

    // Authenticate connection (simplified for PoC)
    const token = this.extractTokenFromRequest(request);
    if (!this.validateToken(token)) {
      console.log(`🚫 Unauthorized connection attempt: ${clientId}`);
      ws.close(1008, 'Unauthorized');
      return;
    }

    this.clients.set(clientId, { ws, ...clientInfo });
    this.stats.totalConnections++;
    this.stats.currentConnections++;

    // Send welcome message
    this.sendToClient(clientId, {
      type: 'connection:established',
      clientId,
      timestamp: Date.now(),
      message: 'Connected to Odin WebSocket server'
    });

    // Handle client messages
    ws.on('message', (data) => {
      try {
        const message = JSON.parse(data.toString());
        this.handleClientMessage(clientId, message);
      } catch (error) {
        console.error(`❌ Invalid message from ${clientId}:`, error);
      }
    });

    // Handle disconnection
    ws.on('close', (code, reason) => {
      console.log(`🔌 Client disconnected: ${clientId} (${code}: ${reason})`);
      this.clients.delete(clientId);
      this.stats.currentConnections--;
    });

    // Handle errors
    ws.on('error', (error) => {
      console.error(`❌ WebSocket error for ${clientId}:`, error);
    });

    // Send heartbeat
    this.startHeartbeat(clientId);
  }

  extractTokenFromRequest(request) {
    const url = new URL(request.url, 'http://localhost');
    return url.searchParams.get('token') || request.headers.authorization?.split(' ')[1];
  }

  validateToken(token) {
    if (!token) return config.env === 'development'; // Allow no auth in dev mode

    try {
      jwt.verify(token, config.jwt.secret);
      return true;
    } catch {
      return false;
    }
  }

  handleClientMessage(clientId, message) {
    console.log(`📨 Message from ${clientId}:`, message.type);

    switch (message.type) {
      case 'ping':
        this.sendToClient(clientId, {
          type: 'pong',
          timestamp: Date.now(),
          originalTimestamp: message.timestamp
        });
        break;

      case 'subscribe':
        // Handle subscription requests (future enhancement)
        this.sendToClient(clientId, {
          type: 'subscription:ack',
          subjects: message.subjects || ['all'],
          timestamp: Date.now()
        });
        break;

      default:
        console.log(`⚠️  Unknown message type: ${message.type}`);
    }
  }

  sendToClient(clientId, data) {
    const client = this.clients.get(clientId);
    if (client?.ws.readyState === 1) { // WebSocket.OPEN
      client.ws.send(JSON.stringify(data));
      this.stats.messagesSent++;
      return true;
    }
    return false;
  }

  broadcastToClients(data) {
    let sentCount = 0;
    for (const [clientId, client] of this.clients) {
      if (this.sendToClient(clientId, data)) {
        sentCount++;
      }
    }

    if (sentCount > 0) {
      console.log(`📢 Broadcasted ${data.type} to ${sentCount} clients`);
    }

    return sentCount;
  }

  startHeartbeat(clientId) {
    const client = this.clients.get(clientId);
    if (!client) return;

    const heartbeatInterval = setInterval(() => {
      if (!this.clients.has(clientId)) {
        clearInterval(heartbeatInterval);
        return;
      }

      this.sendToClient(clientId, {
        type: 'heartbeat',
        timestamp: Date.now(),
        connectedClients: this.stats.currentConnections
      });
    }, 30000); // 30 seconds

    client.heartbeatInterval = heartbeatInterval;
  }

  startHealthCheck() {
    setInterval(() => {
      // Clean up dead connections
      for (const [clientId, client] of this.clients) {
        if (client.ws.readyState !== 1) { // Not OPEN
          console.log(`🧹 Cleaning up dead connection: ${clientId}`);
          this.clients.delete(clientId);
          this.stats.currentConnections--;
        }
      }
    }, 60000); // 1 minute
  }

  generateClientId() {
    return `client_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
  }

  async shutdown() {
    console.log('🛑 Shutting down server...');

    // Close all WebSocket connections
    for (const [clientId, client] of this.clients) {
      client.ws.close(1000, 'Server shutdown');
      if (client.heartbeatInterval) {
        clearInterval(client.heartbeatInterval);
      }
    }

    // Close NATS connection
    if (this.nats && !this.nats.isClosed()) {
      await this.nats.close();
    }

    // Close HTTP server
    this.server.close();
    console.log('✅ Server shutdown complete');
  }
}

// Start server
const server = new OdinWebSocketServer();

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
  console.error('❌ Failed to start server:', error);
  process.exit(1);
});