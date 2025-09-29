#!/usr/bin/env tsx
import jwt from 'jsonwebtoken';
import { config } from '../config/odin.config';
export class AuthTokenGenerator {
    static generate(userId = 'load-test-user', expiryHours = 24) {
        const now = Math.floor(Date.now() / 1000);
        const payload = {
            userId,
            iat: now,
            exp: now + (expiryHours * 60 * 60)
        };
        return jwt.sign(payload, config.jwt.secret);
    }
    static validate(token) {
        try {
            return jwt.verify(token, config.jwt.secret);
        }
        catch (error) {
            console.error('Token validation failed:', error);
            return null;
        }
    }
    static decode(token) {
        try {
            return jwt.decode(token);
        }
        catch (error) {
            console.error('Token decode failed:', error);
            return null;
        }
    }
    static printToken(userId = 'load-test-user', expiryHours = 24) {
        const token = this.generate(userId, expiryHours);
        const decoded = this.decode(token);
        console.log('🔑 Generated JWT Token');
        console.log('==========================================');
        console.log(`User ID: ${decoded?.userId}`);
        console.log(`Issued At: ${new Date((decoded?.iat || 0) * 1000).toISOString()}`);
        console.log(`Expires At: ${new Date((decoded?.exp || 0) * 1000).toISOString()}`);
        console.log('');
        console.log('Token:');
        console.log(token);
        console.log('');
        console.log('💡 Usage Examples:');
        console.log(`export AUTH_TOKEN="${token}"`);
        console.log(`WS_URL="ws://localhost:8080/ws?token=${token}"`);
        console.log(`curl -H "Authorization: Bearer ${token}" http://localhost:3001/health`);
    }
}
// CLI interface
async function main() {
    const userId = process.argv[2] || 'load-test-user';
    const expiryHours = parseInt(process.argv[3] || '24');
    if (process.argv.includes('--help') || process.argv.includes('-h')) {
        console.log('🔑 JWT Token Generator');
        console.log('');
        console.log('Usage:');
        console.log('  tsx src/utils/auth-token.ts [userId] [expiryHours]');
        console.log('');
        console.log('Examples:');
        console.log('  tsx src/utils/auth-token.ts                    # Default user, 24h expiry');
        console.log('  tsx src/utils/auth-token.ts test-user          # Custom user, 24h expiry');
        console.log('  tsx src/utils/auth-token.ts test-user 48       # Custom user, 48h expiry');
        console.log('');
        process.exit(0);
    }
    try {
        AuthTokenGenerator.printToken(userId, expiryHours);
    }
    catch (error) {
        console.error('❌ Failed to generate token:', error);
        process.exit(1);
    }
}
// Run if called directly
if (import.meta.url === `file://${process.argv[1]}`) {
    main();
}
//# sourceMappingURL=auth-token.js.map