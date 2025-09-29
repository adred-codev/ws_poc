export class ClientId {
    _value;
    constructor(value) {
        if (!value || value.trim().length === 0) {
            throw new Error('ClientId cannot be empty');
        }
        this._value = value.trim();
    }
    get value() {
        return this._value;
    }
    equals(other) {
        return this._value === other._value;
    }
    toString() {
        return this._value;
    }
    static generate() {
        const timestamp = Date.now();
        const random = Math.random().toString(36).substr(2, 9);
        return new ClientId(`client_${timestamp}_${random}`);
    }
}
//# sourceMappingURL=ClientId.js.map