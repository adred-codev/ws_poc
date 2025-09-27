import { ClientId } from '../value-objects/ClientId';
import { Nonce } from '../value-objects/Nonce';

export class Client {
  private readonly _id: ClientId;
  private readonly _connectedAt: Date;
  private readonly _ip: string;
  private readonly _userAgent: string;
  private _messageCount: number = 0;
  private _lastActivity: Date;
  private readonly _seenNonces: Set<string> = new Set();

  constructor(
    id: ClientId,
    ip: string,
    userAgent: string,
    connectedAt: Date = new Date()
  ) {
    this._id = id;
    this._ip = ip;
    this._userAgent = userAgent;
    this._connectedAt = connectedAt;
    this._lastActivity = connectedAt;
  }

  get id(): ClientId {
    return this._id;
  }

  get connectedAt(): Date {
    return this._connectedAt;
  }

  get ip(): string {
    return this._ip;
  }

  get userAgent(): string {
    return this._userAgent;
  }

  get messageCount(): number {
    return this._messageCount;
  }

  get lastActivity(): Date {
    return this._lastActivity;
  }

  get connectionDuration(): number {
    return Date.now() - this._connectedAt.getTime();
  }

  incrementMessageCount(): void {
    this._messageCount++;
    this._lastActivity = new Date();
  }

  hasSeenNonce(nonce: Nonce): boolean {
    return this._seenNonces.has(nonce.value);
  }

  addSeenNonce(nonce: Nonce): void {
    this._seenNonces.add(nonce.value);
  }

  isActive(timeoutMs: number = 30000): boolean {
    return Date.now() - this._lastActivity.getTime() < timeoutMs;
  }

  updateActivity(): void {
    this._lastActivity = new Date();
  }
}