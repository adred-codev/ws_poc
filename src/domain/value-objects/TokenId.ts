export class TokenId {
  private readonly _value: string;

  constructor(value: string) {
    if (!value || value.trim().length === 0) {
      throw new Error('TokenId cannot be empty');
    }

    // Validate token format (letters/numbers only, 2-10 chars)
    if (!/^[A-Z0-9]{2,10}$/i.test(value.trim())) {
      throw new Error('TokenId must be 2-10 alphanumeric characters');
    }

    this._value = value.trim().toUpperCase();
  }

  get value(): string {
    return this._value;
  }

  equals(other: TokenId): boolean {
    return this._value === other._value;
  }

  toString(): string {
    return this._value;
  }

  static fromString(value: string): TokenId {
    return new TokenId(value);
  }
}