import crypto from 'crypto';
export class Nonce {
    _value;
    constructor(value) {
        if (!value || value.trim().length === 0) {
            throw new Error('Nonce cannot be empty');
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
        const randomBytes = crypto.randomBytes(8).toString('hex');
        return new Nonce(`${timestamp}-${randomBytes}`);
    }
    static fromString(value) {
        return new Nonce(value);
    }
}
//# sourceMappingURL=Nonce.js.map