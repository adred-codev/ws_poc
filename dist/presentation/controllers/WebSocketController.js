import { WebSocket } from 'ws';
export class WebSocketController {
    connectionService;
    messageService;
    webSocketRepository;
    constructor(connectionService, messageService, webSocketRepository) {
        this.connectionService = connectionService;
        this.messageService = messageService;
        this.webSocketRepository = webSocketRepository;
    }
    async handleConnection(socket, request) {
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
        }
        catch (error) {
            console.error('❌ Error handling WebSocket connection:', error);
            socket.close(1011, 'Internal server error');
        }
    }
    setupMessageHandlers(socket, clientId) {
        socket.on('message', async (data) => {
            try {
                const message = JSON.parse(data.toString());
                await this.handleClientMessage(clientId, message);
            }
            catch (error) {
                console.error(`❌ Invalid message from ${clientId.value}:`, error);
            }
        });
        socket.on('error', (error) => {
            console.error(`❌ WebSocket error for ${clientId.value}:`, error);
        });
    }
    setupDisconnectHandler(socket, clientId) {
        socket.on('close', async (code, reason) => {
            console.log(`🔌 Client disconnected: ${clientId.value} (${code}: ${reason.toString()})`);
            // Clean up
            this.webSocketRepository.removeConnection(clientId);
            await this.connectionService.disconnectClient(clientId);
        });
    }
    async handleClientMessage(clientId, message) {
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
    async handlePing(clientId, message) {
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
    async handleSubscription(clientId, message) {
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
    extractTokenFromRequest(request) {
        const url = new URL(request.url || '', 'http://localhost');
        return url.searchParams.get('token') || request.headers.authorization?.split(' ')[1] || undefined;
    }
    startHeartbeat(clientId) {
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
                }
                else {
                    clearInterval(heartbeatInterval);
                }
            }
            catch (error) {
                console.error('❌ Error sending heartbeat:', error);
                clearInterval(heartbeatInterval);
            }
        }, 30000); // 30 seconds
    }
}
//# sourceMappingURL=WebSocketController.js.map