import { WebSocket } from 'ws';
import { IncomingMessage } from 'http';
import { ConnectionService } from '../../application/services/ConnectionService.js';
import { MessageService } from '../../application/services/MessageService.js';
import { ClientId } from '../../domain/value-objects/ClientId.js';
import { WebSocketMessageRepository } from '../../infrastructure/websocket/WebSocketMessageRepository.js';

export interface ClientMessage {
  type: string;
  timestamp?: number;
  [key: string]: any;
}

export class WebSocketController {
  constructor(
    private readonly connectionService: ConnectionService,
    private readonly messageService: MessageService,
    private readonly webSocketRepository: WebSocketMessageRepository
  ) {}

  async handleConnection(socket: WebSocket, request: IncomingMessage): Promise<void> {
    try {
      // Extract client information
      const ip = request.socket.remoteAddress || 'unknown';
      const userAgent = request.headers['user-agent'] || 'unknown';
      const token = this.extractTokenFromRequest(request);

      // Connect client using domain service
      const connectionResult = await this.connectionService.connectClient({
        ip,
        userAgent,
        token
      });

      if (!connectionResult.success) {
        socket.close(1008, 'Connection rejected');
        return;
      }

      const clientId = connectionResult.clientId;
      console.log(`🔗 New client connected: ${clientId.value}`);

      // Register WebSocket connection
      this.webSocketRepository.addConnection(clientId, socket);

      // Set up message handlers
      this.setupMessageHandlers(socket, clientId);

      // Set up disconnect handler
      this.setupDisconnectHandler(socket, clientId);

      // Start heartbeat
      this.startHeartbeat(clientId);

    } catch (error) {
      console.error('❌ Error handling WebSocket connection:', error);
      socket.close(1011, 'Internal server error');
    }
  }

  private setupMessageHandlers(socket: WebSocket, clientId: ClientId): void {
    socket.on('message', async (data: Buffer) => {
      try {
        const message: ClientMessage = JSON.parse(data.toString());
        await this.handleClientMessage(clientId, message);
      } catch (error) {
        console.error(`❌ Invalid message from ${clientId.value}:`, error);
      }
    });

    socket.on('error', (error: Error) => {
      console.error(`❌ WebSocket error for ${clientId.value}:`, error);
    });
  }

  private setupDisconnectHandler(socket: WebSocket, clientId: ClientId): void {
    socket.on('close', async (code: number, reason: Buffer) => {
      console.log(`🔌 Client disconnected: ${clientId.value} (${code}: ${reason.toString()})`);

      // Clean up
      this.webSocketRepository.removeConnection(clientId);
      await this.connectionService.disconnectClient(clientId);
    });
  }

  private async handleClientMessage(clientId: ClientId, message: ClientMessage): Promise<void> {
    console.log(`📨 Message from ${clientId.value}:`, message.type);

    // Update client activity
    await this.connectionService.updateClientActivity(clientId);

    switch (message.type) {
      case 'ping':
        await this.handlePing(clientId, message);
        break;

      case 'subscribe':
        await this.handleSubscription(clientId, message);
        break;

      default:
        console.log(`⚠️ Unknown message type: ${message.type}`);
    }
  }

  private async handlePing(clientId: ClientId, message: ClientMessage): Promise<void> {
    const pongMessage = {
      type: 'pong',
      timestamp: Date.now(),
      originalTimestamp: message.timestamp,
      latency: message.timestamp ? Date.now() - message.timestamp : undefined
    };

    // Send pong directly through WebSocket repository
    const socket = this.webSocketRepository['connections'].get(clientId.value);
    if (socket && socket.readyState === WebSocket.OPEN) {
      socket.send(JSON.stringify(pongMessage));
    }
  }

  private async handleSubscription(clientId: ClientId, message: ClientMessage): Promise<void> {
    const subscriptionAck = {
      type: 'subscription:ack',
      subjects: message.subjects || ['all'],
      timestamp: Date.now()
    };

    // Send subscription acknowledgment
    const socket = this.webSocketRepository['connections'].get(clientId.value);
    if (socket && socket.readyState === WebSocket.OPEN) {
      socket.send(JSON.stringify(subscriptionAck));
    }
  }

  private extractTokenFromRequest(request: IncomingMessage): string | undefined {
    const url = new URL(request.url || '', 'http://localhost');
    return url.searchParams.get('token') || request.headers.authorization?.split(' ')[1] || undefined;
  }

  private startHeartbeat(clientId: ClientId): void {
    const heartbeatInterval = setInterval(async () => {
      const client = await this.connectionService.findClient(clientId);
      if (!client) {
        clearInterval(heartbeatInterval);
        return;
      }

      try {
        const connectionMetrics = await this.connectionService.getConnectionMetrics();
        const heartbeatMessage = {
          type: 'heartbeat',
          timestamp: Date.now(),
          connectedClients: connectionMetrics.currentConnections
        };

        const socket = this.webSocketRepository['connections'].get(clientId.value);
        if (socket && socket.readyState === WebSocket.OPEN) {
          socket.send(JSON.stringify(heartbeatMessage));
        } else {
          clearInterval(heartbeatInterval);
        }
      } catch (error) {
        console.error('❌ Error sending heartbeat:', error);
        clearInterval(heartbeatInterval);
      }
    }, 30000); // 30 seconds
  }
}