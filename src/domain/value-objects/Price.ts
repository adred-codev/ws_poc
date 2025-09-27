export class Price {
  private readonly _value: number;

  constructor(value: number) {
    if (value < 0) {
      throw new Error('Price cannot be negative');
    }
    if (!Number.isFinite(value)) {
      throw new Error('Price must be a finite number');
    }
    this._value = value;
  }

  get value(): number {
    return this._value;
  }

  equals(other: Price): boolean {
    return Math.abs(this._value - other._value) < 1e-8;
  }

  toString(): string {
    return this._value.toString();
  }

  toFixed(decimals: number = 2): string {
    return this._value.toFixed(decimals);
  }

  calculateChange(previousPrice: Price): PriceChange {
    const absoluteChange = this._value - previousPrice._value;
    const percentageChange = previousPrice._value === 0
      ? 0
      : (absoluteChange / previousPrice._value) * 100;

    return new PriceChange(absoluteChange, percentageChange);
  }

  static fromNumber(value: number): Price {
    return new Price(value);
  }
}

export class PriceChange {
  constructor(
    private readonly _absolute: number,
    private readonly _percentage: number
  ) {}

  get absolute(): number {
    return this._absolute;
  }

  get percentage(): number {
    return this._percentage;
  }

  isPositive(): boolean {
    return this._absolute > 0;
  }

  isNegative(): boolean {
    return this._absolute < 0;
  }

  toString(): string {
    const sign = this._absolute >= 0 ? '+' : '';
    return `${sign}${this._absolute.toFixed(2)} (${sign}${this._percentage.toFixed(2)}%)`;
  }
}