export declare class Price {
    private readonly _value;
    constructor(value: number);
    get value(): number;
    equals(other: Price): boolean;
    toString(): string;
    toFixed(decimals?: number): string;
    calculateChange(previousPrice: Price): PriceChange;
    static fromNumber(value: number): Price;
}
export declare class PriceChange {
    private readonly _absolute;
    private readonly _percentage;
    constructor(_absolute: number, _percentage: number);
    get absolute(): number;
    get percentage(): number;
    isPositive(): boolean;
    isNegative(): boolean;
    toString(): string;
}
//# sourceMappingURL=Price.d.ts.map