export class Price {
    _value;
    constructor(value) {
        if (value < 0) {
            throw new Error('Price cannot be negative');
        }
        if (!Number.isFinite(value)) {
            throw new Error('Price must be a finite number');
        }
        this._value = value;
    }
    get value() {
        return this._value;
    }
    equals(other) {
        return Math.abs(this._value - other._value) < 1e-8;
    }
    toString() {
        return this._value.toString();
    }
    toFixed(decimals = 2) {
        return this._value.toFixed(decimals);
    }
    calculateChange(previousPrice) {
        const absoluteChange = this._value - previousPrice._value;
        const percentageChange = previousPrice._value === 0
            ? 0
            : (absoluteChange / previousPrice._value) * 100;
        return new PriceChange(absoluteChange, percentageChange);
    }
    static fromNumber(value) {
        return new Price(value);
    }
}
export class PriceChange {
    _absolute;
    _percentage;
    constructor(_absolute, _percentage) {
        this._absolute = _absolute;
        this._percentage = _percentage;
    }
    get absolute() {
        return this._absolute;
    }
    get percentage() {
        return this._percentage;
    }
    isPositive() {
        return this._absolute > 0;
    }
    isNegative() {
        return this._absolute < 0;
    }
    toString() {
        const sign = this._absolute >= 0 ? '+' : '';
        return `${sign}${this._absolute.toFixed(2)} (${sign}${this._percentage.toFixed(2)}%)`;
    }
}
//# sourceMappingURL=Price.js.map