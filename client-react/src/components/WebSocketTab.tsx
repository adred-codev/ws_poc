import { useAtom, useSetAtom } from 'jotai';
import { useEffect, useRef } from 'react';
import { Play, Square, Wifi, WifiOff, Server, RefreshCw } from 'lucide-react';
import {
  webSocketStateAtom,
  updateServerStateAtom,
  addMessageAtom,
} from '../store/atoms';

const MAX_RECONNECT_ATTEMPTS = 5;
const SERVERS = {
  node: { url: 'ws://localhost:3001/ws', label: 'Node.js' },
  go: { url: 'ws://localhost:3002/ws', label: 'Go' }
};

// A hook-like function to manage WebSocket connections
const useWebSocketManager = () => {
  const [webSocketState, setWebSocketState] = useAtom(webSocketStateAtom);
  const setServerState = useSetAtom(updateServerStateAtom);
  const addMessage = useSetAtom(addMessageAtom);

  const connect = (server: 'node' | 'go') => {
    const serverState = webSocketState[server];
    if (serverState.reconnect.timeoutId) {
      clearTimeout(serverState.reconnect.timeoutId);
    }

    setServerState({ server, partialState: { status: 'connecting', isIntentionalDisconnect: false } });

    const websocket = new WebSocket(SERVERS[server].url);

    websocket.onopen = () => {
      setServerState({ server, partialState: { status: 'connected', reconnect: { timeoutId: null, attempt: 0 } } });
      addMessage({ server, type: 'system', data: { message: `Connected to ${SERVERS[server].label} server` } });
    };

    websocket.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        addMessage({ server, type: 'message', data });
      } catch (e) {
        addMessage({ server, type: 'error', data: { message: 'Failed to parse message' } });
      }
    };

    websocket.onerror = (err) => {
      addMessage({ server, type: 'error', data: { message: `Connection error` } });
    };

    websocket.onclose = () => {
      // Get fresh state to check if intentional disconnect
      setWebSocketState((currentState) => {
        const latestState = currentState[server];
        if (latestState.isIntentionalDisconnect) {
          setServerState({ server, partialState: { status: 'disconnected' } });
          addMessage({ server, type: 'system', data: { message: 'Disconnected from server' } });
          return currentState;
        }

        if (latestState.reconnect.attempt < MAX_RECONNECT_ATTEMPTS) {
          const attempt = latestState.reconnect.attempt + 1;
          const delay = Math.min(30000, Math.pow(2, attempt) * 1000);
          setServerState({ server, partialState: { status: 'reconnecting' } });
          addMessage({ server, type: 'system', data: { message: `Connection lost. Reconnecting in ${delay / 1000}s...` } });

          const timeoutId = setTimeout(() => connect(server), delay);
          setServerState({ server, partialState: { reconnect: { timeoutId, attempt } } });
        } else {
          setServerState({ server, partialState: { status: 'disconnected' } });
          addMessage({ server, type: 'error', data: { message: 'Could not reconnect to the server.' } });
        }
        return currentState;
      });
    };

    setServerState({ server, partialState: { ws: websocket } });
  };

  const disconnect = (server: 'node' | 'go') => {
    // First, set the intentional disconnect flag to prevent reconnection
    setServerState({ server, partialState: { isIntentionalDisconnect: true } });
    
    // Get the latest state using the setter callback pattern
    setWebSocketState((currentState) => {
      const serverState = currentState[server];
      
      // Clear any reconnection timeout
      if (serverState.reconnect.timeoutId) {
        clearTimeout(serverState.reconnect.timeoutId);
      }
      
      // Close the WebSocket connection if it exists
      if (serverState.ws && serverState.ws.readyState !== WebSocket.CLOSED) {
        serverState.ws.close(1000, 'User disconnected');
      }
      
      // Update the state to reflect disconnection
      return {
        ...currentState,
        [server]: {
          ...serverState,
          ws: null,
          status: 'disconnected',
          reconnect: { timeoutId: null, attempt: 0 },
          isIntentionalDisconnect: true
        }
      };
    });
  };

  return { connect, disconnect };
};

// Component for individual log pane
const LogPane = ({ server, title }: { server: 'node' | 'go'; title: string }) => {
  const [webSocketState, setWebSocketState] = useAtom(webSocketStateAtom);
  const serverState = webSocketState[server];
  const { status, messages } = serverState;
  const { connect, disconnect } = useWebSocketManager();
  const messagesEndRef = useRef<HTMLDivElement>(null);

  // Remove auto-scroll for now
  // useEffect(() => {
  //   messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  // }, [messages]);

  const handleConnect = () => {
    if (status === 'disconnected') {
      connect(server);
    }
  };

  const handleDisconnect = () => {
    disconnect(server);
  };

  const clearMessages = () => {
    setWebSocketState((prev) => ({
      ...prev,
      [server]: {
        ...prev[server],
        messages: [],
      },
    }));
  };

  return (
    <div className="flex flex-col h-full min-h-0">
      {/* Header with controls */}
      <div className="flex items-center justify-between p-3 bg-gray-800 border-b border-gray-700 flex-shrink-0">
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-2">
            <Server size={18} className="text-gray-400" />
            <span className="text-gray-200 font-medium">{title}</span>
          </div>

          <div className={`flex items-center gap-2 px-2 py-1 rounded-full text-xs ${
            status === 'connected' ? 'bg-green-900 text-green-300' :
            status === 'connecting' ? 'bg-yellow-900 text-yellow-300' :
            status === 'reconnecting' ? 'bg-orange-900 text-orange-300' :
            'bg-red-900 text-red-300'
          }`}>
            {status === 'connected' ? <Wifi size={14} /> :
             status === 'reconnecting' ? <RefreshCw size={14} className="animate-spin" /> :
             <WifiOff size={14} />}
            <span className="font-medium capitalize">{status}</span>
          </div>
        </div>

        <div className="flex items-center gap-2">
          {status === 'connected' || status === 'reconnecting' ? (
            <button
              onClick={handleDisconnect}
              className="flex items-center gap-1 px-3 py-1 bg-red-600 hover:bg-red-700 text-white text-sm rounded transition-colors"
            >
              <Square size={14} />
              Disconnect
            </button>
          ) : (
            <button
              onClick={handleConnect}
              className="flex items-center gap-1 px-3 py-1 bg-green-600 hover:bg-green-700 text-white text-sm rounded transition-colors"
            >
              <Play size={14} />
              Connect
            </button>
          )}

          <button
            onClick={clearMessages}
            className="px-3 py-1 text-gray-400 hover:text-gray-200 text-sm transition-colors"
          >
            Clear
          </button>
        </div>
      </div>

      {/* Message Log - with proper scrolling */}
      <div className="flex-1 bg-gray-900 overflow-hidden min-h-0">
        <div className="h-full overflow-y-auto p-3 font-mono text-xs">
          {messages.length === 0 ? (
            <div className="text-gray-500 text-center py-4">No messages yet. Connect to start receiving messages.</div>
          ) : (
            <div className="space-y-1">
              {messages.map((msg, idx) => (
                <div key={idx} className="flex gap-2">
                  <span className="text-gray-500 shrink-0">{msg.time}</span>
                  <span className={`shrink-0 px-1 py-0.5 rounded text-xs ${
                      msg.type === 'system' ? 'bg-blue-900 text-blue-300' :
                      msg.type === 'error' ? 'bg-red-900 text-red-300' :
                      'bg-gray-700 text-gray-300'
                    }`}>{msg.type}</span>
                  <span className="text-gray-200 break-all">{typeof msg.data === 'object' ? JSON.stringify(msg.data) : msg.data}</span>
                </div>
              ))}
              <div ref={messagesEndRef} />
            </div>
          )}
        </div>
      </div>

      {/* Footer stats */}
      <div className="p-2 bg-gray-800 border-t border-gray-700 flex items-center justify-between text-xs flex-shrink-0">
        <span className="text-gray-400">Messages: <span className="text-gray-200 font-medium">{messages.length}</span></span>
        <span className="text-gray-400">URL: <span className="text-gray-200 font-medium">{server === 'node' ? 'ws://localhost:3001/ws' : 'ws://localhost:3002/ws'}</span></span>
      </div>
    </div>
  );
};

function WebSocketTab() {
  const [webSocketState] = useAtom(webSocketStateAtom);
  const { connect, disconnect } = useWebSocketManager();

  const handleConnectAll = () => {
    connect('node');
    connect('go');
  };

  const handleDisconnectAll = () => {
    disconnect('node');
    disconnect('go');
  };

  return (
    <div className="flex flex-col space-y-4" style={{ height: 'calc(75vh + 300px)' }}>
      {/* Global Controls Header */}
      <div className="flex items-center justify-between p-4 bg-gray-800 rounded-lg flex-shrink-0">
        <div className="flex items-center gap-3">
          <h2 className="text-lg font-semibold text-gray-200">WebSocket Servers</h2>

          <div className="flex gap-2 text-xs">
            <div className={`px-2 py-1 rounded ${webSocketState.node.status === 'connected' ? 'bg-green-900/50 text-green-400' : 'bg-gray-700 text-gray-500'}`}>
              Node: {webSocketState.node.status}
            </div>
            <div className={`px-2 py-1 rounded ${webSocketState.go.status === 'connected' ? 'bg-blue-900/50 text-blue-400' : 'bg-gray-700 text-gray-500'}`}>
              Go: {webSocketState.go.status}
            </div>
          </div>
        </div>

        <div className="flex items-center gap-2">
          <button
            onClick={handleConnectAll}
            className="flex items-center gap-2 px-4 py-2 bg-green-600 hover:bg-green-700 text-white rounded-md transition-colors"
            title="Connect to all servers"
          >
            <Wifi size={16} />
            Connect All
          </button>

          <button
            onClick={handleDisconnectAll}
            className="flex items-center gap-2 px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-md transition-colors"
            title="Disconnect all servers"
          >
            <WifiOff size={16} />
            Disconnect All
          </button>
        </div>
      </div>

      {/* Dual Log Panes - with fixed height for scrolling */}
      <div className="flex-1 grid grid-rows-2 gap-4 overflow-hidden" style={{ minHeight: 0 }}>
        {/* Node.js Server Log Pane */}
        <div className="rounded-lg overflow-hidden border border-gray-700 bg-gray-900" style={{ minHeight: 0 }}>
          <LogPane server="node" title="Node.js Server" />
        </div>

        {/* Go Server Log Pane */}
        <div className="rounded-lg overflow-hidden border border-gray-700 bg-gray-900" style={{ minHeight: 0 }}>
          <LogPane server="go" title="Go Server" />
        </div>
      </div>

      {/* Summary Stats */}
      <div className="grid grid-cols-4 gap-4 text-sm flex-shrink-0">
        <div className="bg-gray-700 rounded-lg p-3">
          <div className="text-gray-400 mb-1">Node.js Messages</div>
          <div className="text-xl font-bold text-green-400">{webSocketState.node.messages.length}</div>
        </div>
        <div className="bg-gray-700 rounded-lg p-3">
          <div className="text-gray-400 mb-1">Go Messages</div>
          <div className="text-xl font-bold text-blue-400">{webSocketState.go.messages.length}</div>
        </div>
        <div className="bg-gray-700 rounded-lg p-3">
          <div className="text-gray-400 mb-1">Total Messages</div>
          <div className="text-xl font-bold text-purple-400">
            {webSocketState.node.messages.length + webSocketState.go.messages.length}
          </div>
        </div>
        <div className="bg-gray-700 rounded-lg p-3">
          <div className="text-gray-400 mb-1">Active Connections</div>
          <div className="text-xl font-bold text-yellow-400">
            {[webSocketState.node.status, webSocketState.go.status].filter(s => s === 'connected').length} / 2
          </div>
        </div>
      </div>
    </div>
  );
}

export default WebSocketTab;