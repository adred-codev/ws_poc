export class InMemoryClientRepository {
    clients = new Map();
    async save(client) {
        this.clients.set(client.id.value, client);
    }
    async findById(id) {
        return this.clients.get(id.value) || null;
    }
    async findAll() {
        return Array.from(this.clients.values());
    }
    async remove(id) {
        this.clients.delete(id.value);
    }
    async count() {
        return this.clients.size;
    }
    async findActiveClients(timeoutMs = 30000) {
        const allClients = Array.from(this.clients.values());
        return allClients.filter(client => client.isActive(timeoutMs));
    }
    async cleanup() {
        // Remove inactive clients older than 5 minutes
        const fiveMinutesAgo = Date.now() - (5 * 60 * 1000);
        const clientsToRemove = [];
        for (const [id, client] of this.clients) {
            if (client.lastActivity.getTime() < fiveMinutesAgo) {
                clientsToRemove.push(id);
            }
        }
        for (const id of clientsToRemove) {
            this.clients.delete(id);
        }
    }
}
//# sourceMappingURL=InMemoryClientRepository.js.map