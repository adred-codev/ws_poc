export declare class Nonce {
    private readonly _value;
    constructor(value: string);
    get value(): string;
    equals(other: Nonce): boolean;
    toString(): string;
    static generate(): Nonce;
    static fromString(value: string): Nonce;
}
//# sourceMappingURL=Nonce.d.ts.map