# Stage 1: Build TypeScript
FROM node:22-alpine AS builder

WORKDIR /app

# Copy package files (context is already publisher directory)
COPY package*.json ./

# Install dependencies
RUN npm ci

# Copy source code (context is already publisher directory)
COPY . .

# Build TypeScript
RUN npm run build

# Stage 2: Runtime
FROM node:22-alpine

WORKDIR /app

# Copy built files
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/package*.json ./

# Install production dependencies only
RUN npm ci --only=production

# Expose HTTP control API port
EXPOSE 3003

# Set environment
ENV NODE_ENV=production
ENV PORT=3003

# Run publisher
CMD ["node", "dist/index.js"]
