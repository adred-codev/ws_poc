#!/usr/bin/env tsx
interface TokenPayload {
    userId: string;
    iat: number;
    exp: number;
}
export declare class AuthTokenGenerator {
    static generate(userId?: string, expiryHours?: number): string;
    static validate(token: string): TokenPayload | null;
    static decode(token: string): TokenPayload | null;
    static printToken(userId?: string, expiryHours?: number): void;
}
export {};
//# sourceMappingURL=auth-token.d.ts.map