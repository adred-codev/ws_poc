export class Client {
    _id;
    _connectedAt;
    _ip;
    _userAgent;
    _messageCount = 0;
    _lastActivity;
    _seenNonces = new Set();
    constructor(id, ip, userAgent, connectedAt = new Date()) {
        this._id = id;
        this._ip = ip;
        this._userAgent = userAgent;
        this._connectedAt = connectedAt;
        this._lastActivity = connectedAt;
    }
    get id() {
        return this._id;
    }
    get connectedAt() {
        return this._connectedAt;
    }
    get ip() {
        return this._ip;
    }
    get userAgent() {
        return this._userAgent;
    }
    get messageCount() {
        return this._messageCount;
    }
    get lastActivity() {
        return this._lastActivity;
    }
    get connectionDuration() {
        return Date.now() - this._connectedAt.getTime();
    }
    incrementMessageCount() {
        this._messageCount++;
        this._lastActivity = new Date();
    }
    hasSeenNonce(nonce) {
        return this._seenNonces.has(nonce.value);
    }
    addSeenNonce(nonce) {
        this._seenNonces.add(nonce.value);
    }
    isActive(timeoutMs = 30000) {
        return Date.now() - this._lastActivity.getTime() < timeoutMs;
    }
    updateActivity() {
        this._lastActivity = new Date();
    }
}
//# sourceMappingURL=Client.js.map