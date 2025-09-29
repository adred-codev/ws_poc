import { Client } from '../entities/Client';
import { ClientId } from '../value-objects/ClientId';
import { MessageType } from '../../types/odin.types';
import { Message } from '../entities/Message';
export class ConnectClientUseCase {
    clientRepository;
    messageRepository;
    constructor(clientRepository, messageRepository) {
        this.clientRepository = clientRepository;
        this.messageRepository = messageRepository;
    }
    async execute(request) {
        try {
            // Generate unique client ID
            const clientId = ClientId.generate();
            // Create client entity
            const client = new Client(clientId, request.ip, request.userAgent);
            // Save client
            await this.clientRepository.save(client);
            // Send welcome message
            await this.sendWelcomeMessage(clientId);
            return {
                clientId,
                success: true,
                message: 'Connected to Odin WebSocket server'
            };
        }
        catch (error) {
            throw new Error(`Failed to connect client: ${error}`);
        }
    }
    async sendWelcomeMessage(clientId) {
        const welcomeMessage = new WelcomeMessage(clientId);
        await this.messageRepository.publishToClient(clientId, welcomeMessage);
    }
}
class WelcomeMessage extends Message {
    _clientId;
    constructor(_clientId, timestamp) {
        super(MessageType.CONNECTION_ESTABLISHED, timestamp);
        this._clientId = _clientId;
    }
    get clientId() {
        return this._clientId;
    }
    toJSON() {
        return {
            type: this._type,
            clientId: this._clientId.value,
            timestamp: this._timestamp.getTime(),
            message: 'Connected to Odin WebSocket server',
            nonce: this._nonce.value
        };
    }
}
//# sourceMappingURL=ConnectClientUseCase.js.map