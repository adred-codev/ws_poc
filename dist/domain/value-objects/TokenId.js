export class TokenId {
    _value;
    constructor(value) {
        if (!value || value.trim().length === 0) {
            throw new Error('TokenId cannot be empty');
        }
        // Validate token format (letters/numbers only, 2-10 chars)
        if (!/^[A-Z0-9]{2,10}$/i.test(value.trim())) {
            throw new Error('TokenId must be 2-10 alphanumeric characters');
        }
        this._value = value.trim().toUpperCase();
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
    static fromString(value) {
        return new TokenId(value);
    }
}
//# sourceMappingURL=TokenId.js.map