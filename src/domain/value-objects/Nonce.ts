import crypto from 'crypto';

export class Nonce {
  private readonly _value: string;

  constructor(value: string) {
    if (!value || value.trim().length === 0) {
      throw new Error('Nonce cannot be empty');
    }
    this._value = value.trim();
  }

  get value(): string {
    return this._value;
  }

  equals(other: Nonce): boolean {
    return this._value === other._value;
  }

  toString(): string {
    return this._value;
  }

  static generate(): Nonce {
    const timestamp = Date.now();
    const randomBytes = crypto.randomBytes(8).toString('hex');
    return new Nonce(`${timestamp}-${randomBytes}`);
  }

  static fromString(value: string): Nonce {
    return new Nonce(value);
  }
}