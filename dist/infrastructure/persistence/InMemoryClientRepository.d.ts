import { Client } from '../../domain/entities/Client';
import { ClientId } from '../../domain/value-objects/ClientId';
import { IClientRepository } from '../../domain/repositories/IClientRepository';
export declare class InMemoryClientRepository implements IClientRepository {
    private readonly clients;
    save(client: Client): Promise<void>;
    findById(id: ClientId): Promise<Client | null>;
    findAll(): Promise<Client[]>;
    remove(id: ClientId): Promise<void>;
    count(): Promise<number>;
    findActiveClients(timeoutMs?: number): Promise<Client[]>;
    cleanup(): Promise<void>;
}
//# sourceMappingURL=InMemoryClientRepository.d.ts.map