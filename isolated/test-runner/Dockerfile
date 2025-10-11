# Test Runner - Isolated Setup
# Lightweight Node.js container for running WebSocket load tests remotely

FROM node:20-alpine

WORKDIR /app

# Install WebSocket client library
RUN npm install ws

# Copy all test scripts
COPY sustained-load-test.cjs /app/
COPY test-connection-rate.cjs /app/
COPY test-message-throughput.cjs /app/

# Set executable permissions
RUN chmod +x *.cjs

# Default command (will be overridden by docker run)
CMD ["node", "sustained-load-test.cjs"]
