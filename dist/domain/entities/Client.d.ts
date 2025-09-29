import { ClientId } from '../value-objects/ClientId';
import { Nonce } from '../value-objects/Nonce';
export declare class Client {
    private readonly _id;
    private readonly _connectedAt;
    private readonly _ip;
    private readonly _userAgent;
    private _messageCount;
    private _lastActivity;
    private readonly _seenNonces;
    constructor(id: ClientId, ip: string, userAgent: string, connectedAt?: Date);
    get id(): ClientId;
    get connectedAt(): Date;
    get ip(): string;
    get userAgent(): string;
    get messageCount(): number;
    get lastActivity(): Date;
    get connectionDuration(): number;
    incrementMessageCount(): void;
    hasSeenNonce(nonce: Nonce): boolean;
    addSeenNonce(nonce: Nonce): void;
    isActive(timeoutMs?: number): boolean;
    updateActivity(): void;
}
//# sourceMappingURL=Client.d.ts.map