import { ConnectClientUseCase } from '../../domain/use-cases/ConnectClientUseCase';
export class ConnectionService {
    clientRepository;
    messageRepository;
    connectClientUseCase;
    constructor(clientRepository, messageRepository) {
        this.clientRepository = clientRepository;
        this.messageRepository = messageRepository;
        this.connectClientUseCase = new ConnectClientUseCase(clientRepository, messageRepository);
    }
    async connectClient(request) {
        return await this.connectClientUseCase.execute(request);
    }
    async disconnectClient(clientId) {
        await this.clientRepository.remove(clientId);
    }
    async findClient(clientId) {
        return await this.clientRepository.findById(clientId);
    }
    async getConnectionMetrics() {
        const allClients = await this.clientRepository.findAll();
        const activeClients = await this.clientRepository.findActiveClients();
        const totalDuration = allClients.reduce((sum, client) => sum + client.connectionDuration, 0);
        const averageConnectionDuration = allClients.length > 0
            ? totalDuration / allClients.length
            : 0;
        return {
            totalConnections: allClients.length,
            currentConnections: allClients.length,
            averageConnectionDuration,
            activeClients: activeClients.length
        };
    }
    async updateClientActivity(clientId) {
        const client = await this.clientRepository.findById(clientId);
        if (client) {
            client.updateActivity();
            await this.clientRepository.save(client);
        }
    }
    async cleanupInactiveClients(timeoutMs = 60000) {
        const allClients = await this.clientRepository.findAll();
        let cleanedUp = 0;
        for (const client of allClients) {
            if (!client.isActive(timeoutMs)) {
                await this.clientRepository.remove(client.id);
                cleanedUp++;
            }
        }
        return cleanedUp;
    }
    async getAllClients() {
        return await this.clientRepository.findAll();
    }
    async getActiveClients(timeoutMs) {
        return await this.clientRepository.findActiveClients(timeoutMs);
    }
}
//# sourceMappingURL=ConnectionService.js.map