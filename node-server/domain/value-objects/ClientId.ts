export class ClientId {
  private readonly _value: string;

  constructor(value: string) {
    if (!value || value.trim().length === 0) {
      throw new Error('ClientId cannot be empty');
    }
    this._value = value.trim();
  }

  get value(): string {
    return this._value;
  }

  equals(other: ClientId): boolean {
    return this._value === other._value;
  }

  toString(): string {
    return this._value;
  }

  static generate(): ClientId {
    const timestamp = Date.now();
    const random = Math.random().toString(36).substr(2, 9);
    return new ClientId(`client_${timestamp}_${random}`);
  }
}