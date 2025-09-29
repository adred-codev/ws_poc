import { WebSocket } from 'ws';
import { ClientId } from '../../domain/value-objects/ClientId';
export class WebSocketMessageRepository {
    clientRepository;
    connections = new Map();
    constructor(clientRepository) {
        this.clientRepository = clientRepository;
    }
    addConnection(clientId, socket) {
        this.connections.set(clientId.value, socket);
    }
    removeConnection(clientId) {
        this.connections.delete(clientId.value);
    }
    async publishToClient(clientId, message) {
        const socket = this.connections.get(clientId.value);
        if (!socket || socket.readyState !== WebSocket.OPEN) {
            return false;
        }
        try {
            const messageData = JSON.stringify(message.toJSON());
            socket.send(messageData);
            // Update client activity
            const client = await this.clientRepository.findById(clientId);
            if (client) {
                client.incrementMessageCount();
                await this.clientRepository.save(client);
            }
            return true;
        }
        catch (error) {
            console.error(`Failed to send message to client ${clientId.value}:`, error);
            return false;
        }
    }
    async broadcastToAllClients(message) {
        const allClientIds = Array.from(this.connections.keys()).map(id => new ClientId(id));
        return await this.broadcastToClients(allClientIds, message);
    }
    async broadcastToClients(clientIds, message) {
        let sentCount = 0;
        for (const clientId of clientIds) {
            const success = await this.publishToClient(clientId, message);
            if (success) {
                sentCount++;
            }
        }
        return sentCount;
    }
    getActiveConnections() {
        const connections = [];
        for (const [clientIdValue, socket] of this.connections) {
            if (socket.readyState === WebSocket.OPEN) {
                connections.push({
                    clientId: new ClientId(clientIdValue),
                    socket
                });
            }
        }
        return connections;
    }
    cleanupDeadConnections() {
        let cleaned = 0;
        for (const [clientIdValue, socket] of this.connections) {
            if (socket.readyState !== WebSocket.OPEN) {
                this.connections.delete(clientIdValue);
                cleaned++;
            }
        }
        return cleaned;
    }
    getConnectionCount() {
        return this.connections.size;
    }
}
//# sourceMappingURL=WebSocketMessageRepository.js.map