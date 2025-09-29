import express from 'express';
import { WebSocketServer, WebSocket } from 'ws';
import { connect, StringCodec } from 'nats';
import jwt from 'jsonwebtoken';
import { createServer } from 'http';
import cors from 'cors';
import crypto from 'crypto';
import { config, subjects, migrationConfig, metricsConfig } from './config/odin.config';
import { MessageType } from './types/odin.types';
import { metricsService } from './services/metrics-service';
import { enhancedMetricsService } from './services/enhanced-metrics';
import metricsRoutes from './routes/metrics-routes';
import path from 'path';
import { fileURLToPath } from 'url';
const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const sc = StringCodec();
class OdinWebSocketServer {
    app;
    server;
    wss = null;
    nats = null;
    clients = new Map();
    connectionState = new Map();
    nonceCache = new Map();
    // Metrics tracking
    metrics = {
        messagesPublished: 0,
        messagesDelivered: 0,
        connectionCount: 0,
        duplicatesDropped: 0,
        reconnectionCount: 0,
        apiRequestsSaved: 0,
        bandwidthSaved: 0,
        latencySum: 0,
        latencyCount: 0,
        peakLatency: 0,
        errors: 0,
        startTime: Date.now()
    };
    // Cache for polling endpoint
    dataCache = new Map();
    constructor() {
        this.app = express();
        this.server = createServer(this.app);
    }
    async initialize() {
        await this.setupNATS();
        this.setupHTTPServer();
        this.setupWebSocketServer();
        await this.setupMetricsIntegration();
        this.startHealthCheck();
        this.startMetricsReporting();
        this.startNonceCleaner();
        console.log('🚀 Odin WebSocket Server initialized with production features');
    }
    async setupMetricsIntegration() {
        // Start enhanced metrics collection
        await enhancedMetricsService.start();
        // Set component status for NATS
        metricsService.setComponentStatus('nats', this.nats?.isClosed() ? 'disconnected' : 'connected');
        metricsService.setComponentStatus('websocket', 'healthy');
        metricsService.setComponentStatus('publisher', 'active');
        // Set up NATS connection monitoring
        if (this.nats) {
            this.nats.closed().then(() => {
                metricsService.setComponentStatus('nats', 'disconnected');
            });
        }
        console.log('📊 Metrics service integration complete');
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
            this.subscribeToSubjects();
        }
        catch (error) {
            console.error('❌ NATS connection failed:', error);
            process.exit(1);
        }
    }
    subscribeToSubjects() {
        if (!this.nats)
            return;
        // Subscribe to all token updates
        const tokenSubscription = this.nats.subscribe('odin.token.*.>');
        this.handleSubscription(tokenSubscription, 'token');
        // Subscribe to batch updates
        const batchSubscription = this.nats.subscribe(subjects.batchUpdate);
        this.handleSubscription(batchSubscription, 'batch');
        // Subscribe to trades
        const tradeSubscription = this.nats.subscribe('odin.trades.*');
        this.handleSubscription(tradeSubscription, 'trade');
        // Subscribe to market stats
        const statsSubscription = this.nats.subscribe(subjects.marketStats);
        this.handleSubscription(statsSubscription, 'stats');
        console.log('📬 Subscribed to Odin subject hierarchy');
    }
    async handleSubscription(subscription, type) {
        (async () => {
            for await (const msg of subscription) {
                const startTime = Date.now();
                try {
                    const data = JSON.parse(sc.decode(msg.data));
                    // Check for duplicate using nonce
                    if (this.isDuplicate(data.nonce)) {
                        this.metrics.duplicatesDropped++;
                        console.log(`🔁 Duplicate dropped: ${data.nonce}`);
                        continue;
                    }
                    // Cache the nonce
                    this.cacheNonce(data.nonce);
                    // Update data cache for polling endpoint
                    this.updateDataCache(type, data);
                    // Broadcast to WebSocket clients
                    const delivered = this.broadcastToClients(data);
                    // Track metrics
                    const latency = Date.now() - startTime;
                    this.updateLatencyMetrics(latency);
                    metricsService.recordLatency(latency);
                    metricsService.recordMessage(true);
                    this.metrics.messagesDelivered += delivered;
                    this.metrics.messagesPublished++;
                }
                catch (error) {
                    console.error(`❌ Error processing ${type} message:`, error);
                    this.metrics.errors++;
                    metricsService.recordError('message_processing', error instanceof Error ? error.message : 'Unknown error');
                    metricsService.recordMessage(false);
                }
            }
        })();
    }
    isDuplicate(nonce) {
        if (!nonce)
            return false;
        return this.nonceCache.has(nonce);
    }
    cacheNonce(nonce) {
        if (!nonce)
            return;
        this.nonceCache.set(nonce, Date.now());
        // Auto-expire after dedup window
        if (config.nats.jetstream?.dedupWindow) {
            setTimeout(() => {
                this.nonceCache.delete(nonce);
            }, config.nats.jetstream.dedupWindow);
        }
    }
    updateDataCache(type, data) {
        // Update cache for polling fallback
        const key = `${type}:${data.tokenId || 'global'}`;
        this.dataCache.set(key, {
            data,
            timestamp: Date.now()
        });
    }
    updateLatencyMetrics(latency) {
        this.metrics.latencySum += latency;
        this.metrics.latencyCount++;
        if (latency > this.metrics.peakLatency) {
            this.metrics.peakLatency = latency;
        }
    }
    setupHTTPServer() {
        this.app.use(cors());
        this.app.use(express.json());
        // Serve metrics dashboard
        this.app.use(express.static(path.join(__dirname, 'public')));
        // Mount metrics routes
        this.app.use('/api', metricsRoutes);
        // Health check endpoint
        this.app.get('/health', (req, res) => {
            const uptime = Date.now() - this.metrics.startTime;
            res.json({
                status: 'healthy',
                uptime: Math.floor(uptime / 1000),
                nats: this.nats?.isClosed() ? 'disconnected' : 'connected',
                websocket: {
                    currentConnections: this.metrics.connectionCount,
                    totalDelivered: this.metrics.messagesDelivered
                },
                duplicatesDropped: this.metrics.duplicatesDropped
            });
        });
        // Metrics endpoint - now includes enhanced metrics
        this.app.get('/metrics', (req, res) => {
            try {
                const enhancedData = enhancedMetricsService.getSimpleMetrics();
                const avgLatency = this.metrics.latencyCount > 0
                    ? (this.metrics.latencySum / this.metrics.latencyCount).toFixed(2)
                    : 0;
                // Combine legacy metrics with enhanced metrics
                const { connectionCount: legacyConnectionCount, ...legacyMetrics } = this.metrics;
                res.json({
                    // Enhanced metrics (accurate CPU, memory, connections)
                    connectionCount: enhancedData.connectionCount,
                    memory: enhancedData.memory,
                    cpu: enhancedData.cpu,
                    // Legacy metrics for compatibility (excluding connectionCount to avoid duplicate)
                    ...legacyMetrics,
                    averageLatency: avgLatency,
                    errorRate: (this.metrics.errors / Math.max(1, this.metrics.messagesPublished)).toFixed(4),
                    uptime: Math.floor((Date.now() - this.metrics.startTime) / 1000)
                });
            }
            catch (error) {
                console.error('Error getting enhanced metrics:', error);
                // Fallback to legacy metrics only
                const avgLatency = this.metrics.latencyCount > 0
                    ? (this.metrics.latencySum / this.metrics.latencyCount).toFixed(2)
                    : 0;
                res.json({
                    ...this.metrics,
                    averageLatency: avgLatency,
                    errorRate: (this.metrics.errors / Math.max(1, this.metrics.messagesPublished)).toFixed(4),
                    uptime: Math.floor((Date.now() - this.metrics.startTime) / 1000),
                    connectionCount: this.metrics.connectionCount,
                    memory: 0,
                    cpu: 0
                });
            }
        });
        // Enhanced metrics endpoint (full detailed metrics)
        this.app.get('/metrics/enhanced', (req, res) => {
            try {
                const enhancedData = enhancedMetricsService.getMetrics();
                res.json(enhancedData);
            }
            catch (error) {
                console.error('Error getting enhanced metrics:', error);
                res.status(500).json({
                    error: 'Enhanced metrics not available',
                    message: error instanceof Error ? error.message : 'Unknown error'
                });
            }
        });
        // Polling endpoint for gradual migration
        this.app.get('/api/poll/:tokenId?', (req, res) => {
            const { tokenId } = req.params;
            // Track API request saved (would have been made in old system)
            this.metrics.apiRequestsSaved++;
            if (tokenId) {
                const data = this.dataCache.get(`token:${tokenId}`);
                res.json(data || { error: 'No data available' });
            }
            else {
                // Return all cached data
                const allData = {};
                for (const [key, value] of this.dataCache) {
                    allData[key] = value;
                }
                res.json(allData);
            }
        });
        // Simulated trade endpoint
        this.app.post('/api/trade', async (req, res) => {
            const { tokenId, userId, side, amount } = req.body;
            // Get current price from cache
            const priceData = this.dataCache.get(`token:${tokenId}`);
            const price = priceData?.data?.price || 0;
            // Create trade message
            const tradeMessage = {
                type: MessageType.TRADE_EXECUTED,
                tradeId: crypto.randomBytes(16).toString('hex'),
                tokenId,
                userId,
                side,
                price,
                amount,
                timestamp: Date.now(),
                nonce: this.generateNonce()
            };
            // Publish to NATS
            if (this.nats) {
                await this.nats.publish(subjects.trades(tokenId), sc.encode(JSON.stringify(tradeMessage)));
                // Also publish immediate price update
                const priceUpdate = {
                    type: MessageType.PRICE_UPDATE,
                    tokenId,
                    price: price * (side === 'buy' ? 1.001 : 0.999), // Slight price impact
                    priceChange24h: 0,
                    percentChange24h: side === 'buy' ? 0.1 : -0.1,
                    volume24h: (priceData?.data?.volume24h || 0) + (price * amount),
                    timestamp: Date.now(),
                    source: 'trade',
                    nonce: this.generateNonce()
                };
                await this.nats.publish(subjects.tokenPrice(tokenId), sc.encode(JSON.stringify(priceUpdate)));
            }
            res.json({
                success: true,
                tradeId: tradeMessage.tradeId,
                executionPrice: price
            });
        });
        // JWT token endpoint
        if (config.env === 'development') {
            this.app.post('/auth/token', (req, res) => {
                const { userId = 'test-user' } = req.body;
                const token = jwt.sign({
                    userId,
                    iat: Math.floor(Date.now() / 1000),
                    exp: Math.floor(Date.now() / 1000) + config.jwt.expiry
                }, config.jwt.secret);
                res.json({ token, userId });
            });
        }
        this.server.listen(config.server.httpPort, () => {
            console.log(`🌐 HTTP server running on port ${config.server.httpPort}`);
            console.log(`📊 Metrics available at http://localhost:${config.server.httpPort}/metrics`);
            console.log(`📈 Dashboard available at http://localhost:${config.server.httpPort}/dashboard.html`);
            if (migrationConfig.enabled) {
                console.log(`🔄 Polling fallback available at /api/poll`);
            }
        });
    }
    setupWebSocketServer() {
        this.wss = new WebSocketServer({
            server: this.server,
            path: migrationConfig.websocketEndpoint
        });
        this.wss.on('connection', (ws, request) => {
            // Check migration rollout percentage
            if (migrationConfig.enabled && Math.random() * 100 > migrationConfig.rolloutPercentage) {
                ws.close(1008, 'WebSocket not available for this user yet');
                return;
            }
            this.handleNewConnection(ws, request);
        });
        console.log(`🔌 WebSocket server ready on port ${config.server.httpPort}${migrationConfig.websocketEndpoint}`);
    }
    handleNewConnection(ws, request) {
        const clientId = this.generateClientId();
        const connectionTime = Date.now();
        const clientInfo = {
            id: clientId,
            connectedAt: connectionTime,
            ip: request.socket.remoteAddress || 'unknown',
            userAgent: request.headers['user-agent'] || 'unknown',
            messageCount: 0,
            lastActivity: connectionTime,
            seenNonces: new Set()
        };
        console.log(`🔗 New client connected: ${clientId}`);
        // Validate JWT token
        const token = this.extractTokenFromRequest(request);
        const tokenData = this.validateToken(token);
        if (!tokenData && config.env !== 'development') {
            console.log(`🚫 Unauthorized connection attempt: ${clientId}`);
            ws.close(1008, 'Unauthorized');
            metricsService.recordConnection(false);
            return;
        }
        // Store client
        this.clients.set(clientId, { ws, ...clientInfo });
        this.connectionState.set(clientId, {
            subscriptions: new Set(['all']), // Default subscription
            preferences: {}
        });
        this.metrics.connectionCount++;
        metricsService.recordConnection(true);
        // Track connection in enhanced metrics
        const remoteAddress = request.socket?.remoteAddress || 'unknown';
        enhancedMetricsService.addConnection(clientId, remoteAddress);
        // Send welcome message
        this.sendToClient(clientId, {
            type: MessageType.CONNECTION_ESTABLISHED,
            clientId,
            timestamp: Date.now(),
            message: 'Connected to Odin WebSocket server',
            rolloutPercentage: migrationConfig.rolloutPercentage,
            features: {
                deduplication: true,
                sourceTracking: true,
                metricsEnabled: true
            },
            nonce: this.generateNonce()
        });
        // Handle client messages
        ws.on('message', (data) => {
            try {
                const message = JSON.parse(data.toString());
                this.handleClientMessage(clientId, message);
            }
            catch (error) {
                console.error(`❌ Invalid message from ${clientId}:`, error);
            }
        });
        // Handle disconnection
        ws.on('close', (code, reason) => {
            console.log(`🔌 Client disconnected: ${clientId} (${code}: ${reason.toString()})`);
            this.clients.delete(clientId);
            this.connectionState.delete(clientId);
            this.metrics.connectionCount--;
            metricsService.recordDisconnection();
            // Remove from enhanced metrics
            enhancedMetricsService.removeConnection(clientId);
        });
        // Handle errors
        ws.on('error', (error) => {
            console.error(`❌ WebSocket error for ${clientId}:`, error);
            this.metrics.errors++;
            metricsService.recordError('websocket', error.message);
        });
        // Start heartbeat
        this.startHeartbeat(clientId);
    }
    extractTokenFromRequest(request) {
        const url = new URL(request.url || '', 'http://localhost');
        return url.searchParams.get('token') || request.headers.authorization?.split(' ')[1] || null;
    }
    validateToken(token) {
        if (!token)
            return null;
        try {
            return jwt.verify(token, config.jwt.secret);
        }
        catch {
            return null;
        }
    }
    handleClientMessage(clientId, message) {
        const client = this.clients.get(clientId);
        if (client) {
            client.lastActivity = Date.now();
            client.messageCount++;
        }
        console.log(`📨 Message from ${clientId}:`, message.type);
        switch (message.type) {
            case MessageType.PING:
                const latency = Date.now() - (message.timestamp || 0);
                this.sendToClient(clientId, {
                    type: MessageType.PONG,
                    timestamp: Date.now(),
                    originalTimestamp: message.timestamp,
                    latency,
                    nonce: this.generateNonce()
                });
                this.updateLatencyMetrics(latency);
                break;
            case MessageType.SUBSCRIBE:
                const state = this.connectionState.get(clientId);
                if (state && message.tokens) {
                    message.tokens.forEach((token) => state.subscriptions.add(token));
                    this.sendToClient(clientId, {
                        type: MessageType.SUBSCRIPTION_ACK,
                        subscriptions: Array.from(state.subscriptions),
                        timestamp: Date.now(),
                        nonce: this.generateNonce()
                    });
                }
                break;
            case MessageType.UNSUBSCRIBE:
                const connState = this.connectionState.get(clientId);
                if (connState && message.tokens) {
                    message.tokens.forEach((token) => connState.subscriptions.delete(token));
                }
                break;
            default:
                console.log(`⚠️  Unknown message type: ${message.type}`);
        }
    }
    sendToClient(clientId, data) {
        const client = this.clients.get(clientId);
        if (client?.ws.readyState === WebSocket.OPEN) {
            // Check client-specific deduplication
            if (data.nonce && client.seenNonces.has(data.nonce)) {
                return false; // Don't send duplicate
            }
            if (data.nonce) {
                client.seenNonces.add(data.nonce);
                // Clean up old nonces periodically
                if (client.seenNonces.size > 1000) {
                    client.seenNonces.clear();
                }
            }
            const messageData = JSON.stringify(data);
            client.ws.send(messageData);
            // Track message in enhanced metrics
            enhancedMetricsService.updateConnectionActivity(clientId, 'sent', messageData.length);
            // Track bandwidth saved (vs polling response)
            this.metrics.bandwidthSaved += 5000 - messageData.length; // Assume 5KB polling response
            return true;
        }
        return false;
    }
    broadcastToClients(data) {
        let sentCount = 0;
        for (const [clientId, client] of this.clients) {
            const state = this.connectionState.get(clientId);
            // Check if client is subscribed to this token
            if (state && data.tokenId) {
                if (!state.subscriptions.has('all') && !state.subscriptions.has(data.tokenId)) {
                    continue; // Skip if not subscribed
                }
            }
            if (this.sendToClient(clientId, data)) {
                sentCount++;
            }
        }
        if (sentCount > 0) {
            console.log(`📢 Broadcasted ${data.type} to ${sentCount} clients (source: ${data.source || 'unknown'})`);
        }
        return sentCount;
    }
    startHeartbeat(clientId) {
        const client = this.clients.get(clientId);
        if (!client)
            return;
        const heartbeatInterval = setInterval(() => {
            if (!this.clients.has(clientId)) {
                clearInterval(heartbeatInterval);
                return;
            }
            const currentClient = this.clients.get(clientId);
            if (!currentClient)
                return;
            const idle = Date.now() - currentClient.lastActivity;
            this.sendToClient(clientId, {
                type: MessageType.HEARTBEAT,
                timestamp: Date.now(),
                connectedClients: this.metrics.connectionCount,
                idleTime: idle,
                nonce: this.generateNonce()
            });
        }, 30000); // 30 seconds
        client.heartbeatInterval = heartbeatInterval;
    }
    startHealthCheck() {
        setInterval(() => {
            // Clean up dead connections
            for (const [clientId, client] of this.clients) {
                if (client.ws.readyState !== WebSocket.OPEN) {
                    console.log(`🧹 Cleaning up dead connection: ${clientId}`);
                    this.clients.delete(clientId);
                    this.connectionState.delete(clientId);
                    this.metrics.connectionCount--;
                }
            }
            // Check for alerts
            if (this.metrics.connectionCount > metricsConfig.thresholds.connections) {
                console.warn(`⚠️  High connection count: ${this.metrics.connectionCount}`);
            }
            const avgLatency = this.metrics.latencyCount > 0
                ? this.metrics.latencySum / this.metrics.latencyCount
                : 0;
            if (avgLatency > metricsConfig.thresholds.latency) {
                console.warn(`⚠️  High latency: ${avgLatency.toFixed(2)}ms`);
            }
        }, 60000); // 1 minute
    }
    startMetricsReporting() {
        setInterval(() => {
            const uptime = Math.floor((Date.now() - this.metrics.startTime) / 1000);
            const avgLatency = this.metrics.latencyCount > 0
                ? (this.metrics.latencySum / this.metrics.latencyCount).toFixed(2)
                : '0';
            console.log(`
📊 Server Metrics:
   Connections: ${this.metrics.connectionCount}
   Messages Delivered: ${this.metrics.messagesDelivered}
   Duplicates Dropped: ${this.metrics.duplicatesDropped}
   Average Latency: ${avgLatency}ms
   Peak Latency: ${this.metrics.peakLatency}ms
   API Requests Saved: ${this.metrics.apiRequestsSaved}
   Bandwidth Saved: ${(this.metrics.bandwidthSaved / 1024 / 1024).toFixed(2)}MB
   Error Rate: ${((this.metrics.errors / Math.max(1, this.metrics.messagesPublished)) * 100).toFixed(2)}%
   Uptime: ${uptime}s
      `);
        }, metricsConfig.reportInterval);
    }
    startNonceCleaner() {
        // Clean expired nonces every minute
        setInterval(() => {
            const now = Date.now();
            const expiry = config.nats.jetstream?.dedupWindow || 60000;
            for (const [nonce, timestamp] of this.nonceCache) {
                if (now - timestamp > expiry) {
                    this.nonceCache.delete(nonce);
                }
            }
            console.log(`🧹 Nonce cache size: ${this.nonceCache.size}`);
        }, 60000);
    }
    generateClientId() {
        return `client_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
    }
    generateNonce() {
        return `${Date.now()}-${crypto.randomBytes(8).toString('hex')}`;
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
//# sourceMappingURL=odin-server.js.map