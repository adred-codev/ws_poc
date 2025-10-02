const WebSocket = require('ws');

console.log('🧪 Testing Go-2 server connection...');

// Test server health first
const testHealth = async () => {
  try {
    const response = await fetch('http://localhost:3004/health');
    const health = await response.json();
    console.log('✅ Health check:', health);
    return true;
  } catch (error) {
    console.log('❌ Health check failed:', error.message);
    return false;
  }
};

// Test WebSocket connection
const testWebSocket = () => {
  return new Promise((resolve) => {
    console.log('🔌 Testing WebSocket connection to ws://localhost:3004/ws');
    
    const ws = new WebSocket('ws://localhost:3004/ws');
    
    ws.on('open', () => {
      console.log('✅ WebSocket connection established');
      ws.close();
      resolve(true);
    });
    
    ws.on('error', (error) => {
      console.log('❌ WebSocket connection failed:', error.message);
      resolve(false);
    });
    
    // Timeout after 5 seconds
    setTimeout(() => {
      console.log('❌ WebSocket connection timeout');
      ws.terminate();
      resolve(false);
    }, 5000);
  });
};

// Run tests
const runTests = async () => {
  console.log('📊 Running Go-2 server tests...\n');
  
  const healthOk = await testHealth();
  if (!healthOk) {
    console.log('\n❌ Server not healthy - check if container is running');
    process.exit(1);
  }
  
  const wsOk = await testWebSocket();
  if (!wsOk) {
    console.log('\n❌ WebSocket connection failed');
    process.exit(1);
  }
  
  console.log('\n✅ All tests passed! Go-2 server is working correctly.');
  console.log('🚀 Ready for stress testing with: node stress-test-high-load.cjs 4000 30 go2');
};

runTests();