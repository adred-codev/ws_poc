import { useAtom } from 'jotai';
import { useEffect, useRef } from 'react';
import { Play, Square, Wifi, WifiOff, Server } from 'lucide-react';
import {
  selectedServerAtom,
  wsConnectionAtom,
  wsStatusAtom,
  wsMessagesAtom
} from '../store/atoms';

function WebSocketTab() {
  const [selectedServer, setSelectedServer] = useAtom(selectedServerAtom);
  const [ws, setWs] = useAtom(wsConnectionAtom);
  const [status, setStatus] = useAtom(wsStatusAtom);
  const [messages, setMessages] = useAtom(wsMessagesAtom);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  const servers = {
    node: { url: 'ws://localhost:3001/ws', label: 'Node.js', color: 'green' },
    go: { url: 'ws://localhost:3002/ws', label: 'Go', color: 'blue' }
  };

  const connect = () => {
    if (ws) {
      ws.close();
    }

    setStatus('connecting');
    const websocket = new WebSocket(servers[selectedServer].url);

    websocket.onopen = () => {
      setStatus('connected');
      addMessage('system', { message: `Connected to ${servers[selectedServer].label} server` });
    };

    websocket.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        addMessage('message', data);
      } catch (e) {
        addMessage('error', { message: 'Failed to parse message' });
      }
    };

    websocket.onerror = () => {
      setStatus('disconnected');
      addMessage('error', { message: 'Connection error' });
    };

    websocket.onclose = () => {
      setStatus('disconnected');
      addMessage('system', { message: 'Disconnected from server' });
    };

    setWs(websocket);
  };

  const disconnect = () => {
    if (ws) {
      ws.close();
      setWs(null);
    }
  };

  const addMessage = (type: string, data: any) => {
    setMessages((prev) => [
      ...prev.slice(-99),
      {
        time: new Date().toLocaleTimeString(),
        type,
        data
      }
    ]);
  };

  const clearMessages = () => {
    setMessages([]);
  };

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <div className="flex items-center gap-2">
            <Server size={20} className="text-gray-400" />
            <select
              value={selectedServer}
              onChange={(e) => setSelectedServer(e.target.value as 'node' | 'go')}
              disabled={status === 'connected'}
              className="bg-gray-700 text-gray-100 px-3 py-2 rounded-md border border-gray-600 focus:border-blue-500 focus:outline-none"
            >
              <option value="node">Node.js Server (:3001)</option>
              <option value="go">Go Server (:3002)</option>
            </select>
          </div>

          <div className="flex items-center gap-2">
            {status === 'connected' ? (
              <button
                onClick={disconnect}
                className="flex items-center gap-2 px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-md transition-colors"
              >
                <Square size={16} />
                Disconnect
              </button>
            ) : (
              <button
                onClick={connect}
                className="flex items-center gap-2 px-4 py-2 bg-green-600 hover:bg-green-700 text-white rounded-md transition-colors"
              >
                <Play size={16} />
                Connect
              </button>
            )}
          </div>
        </div>

        <div className="flex items-center gap-4">
          <div className={`flex items-center gap-2 px-3 py-1 rounded-full ${
            status === 'connected' ? 'bg-green-900 text-green-300' :
            status === 'connecting' ? 'bg-yellow-900 text-yellow-300' :
            'bg-red-900 text-red-300'
          }`}>
            {status === 'connected' ? <Wifi size={16} /> : <WifiOff size={16} />}
            <span className="text-sm font-medium capitalize">{status}</span>
          </div>

          <button
            onClick={clearMessages}
            className="px-3 py-1 text-gray-400 hover:text-gray-200 text-sm"
          >
            Clear Messages
          </button>
        </div>
      </div>

      <div className="bg-gray-900 rounded-lg p-4 h-96 overflow-y-auto font-mono text-sm">
        {messages.length === 0 ? (
          <div className="text-gray-500 text-center py-8">
            No messages yet. Connect to a server to start receiving messages.
          </div>
        ) : (
          <div className="space-y-2">
            {messages.map((msg, idx) => (
              <div key={idx} className="flex gap-3">
                <span className="text-gray-500 shrink-0">{msg.time}</span>
                <span className={`shrink-0 px-2 py-0.5 rounded text-xs ${
                  msg.type === 'system' ? 'bg-blue-900 text-blue-300' :
                  msg.type === 'error' ? 'bg-red-900 text-red-300' :
                  'bg-gray-700 text-gray-300'
                }`}>
                  {msg.type}
                </span>
                <span className="text-gray-200 break-all">
                  {typeof msg.data === 'object' ? JSON.stringify(msg.data) : msg.data}
                </span>
              </div>
            ))}
            <div ref={messagesEndRef} />
          </div>
        )}
      </div>

      <div className="grid grid-cols-3 gap-4 text-sm">
        <div className="bg-gray-700 rounded-lg p-3">
          <div className="text-gray-400 mb-1">Total Messages</div>
          <div className="text-2xl font-bold text-blue-400">{messages.length}</div>
        </div>
        <div className="bg-gray-700 rounded-lg p-3">
          <div className="text-gray-400 mb-1">Server</div>
          <div className="text-2xl font-bold text-green-400">{servers[selectedServer].label}</div>
        </div>
        <div className="bg-gray-700 rounded-lg p-3">
          <div className="text-gray-400 mb-1">Connection</div>
          <div className={`text-2xl font-bold ${
            status === 'connected' ? 'text-green-400' :
            status === 'connecting' ? 'text-yellow-400' :
            'text-red-400'
          }`}>
            {status}
          </div>
        </div>
      </div>
    </div>
  );
}

export default WebSocketTab;