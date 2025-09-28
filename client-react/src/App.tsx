import { useAtom } from 'jotai';
import { Activity, BarChart3, Server } from 'lucide-react';
import { activeTabAtom } from './store/atoms';
import WebSocketTab from './components/WebSocketTab';
import MetricsTab from './components/MetricsTab';
import ContainerTab from './components/ContainerTab';

function App() {
  const [activeTab, setActiveTab] = useAtom(activeTabAtom);

  const tabs = [
    { id: 'websocket', label: 'WebSocket', icon: Activity },
    { id: 'metrics', label: 'Metrics', icon: BarChart3 },
    { id: 'container', label: 'Container Specs', icon: Server },
  ] as const;

  return (
    <div className="min-h-screen bg-gray-900 text-gray-100">
      <div className="container mx-auto px-4 py-6">
        <header className="mb-8">
          <h1 className="text-3xl font-bold bg-gradient-to-r from-blue-400 to-purple-500 bg-clip-text text-transparent">
            Odin WebSocket Monitor
          </h1>
          <p className="text-gray-400 mt-2">Real-time performance monitoring and comparison</p>
        </header>

        <div className="border-b border-gray-700 mb-6">
          <nav className="flex gap-1">
            {tabs.map(({ id, label, icon: Icon }) => (
              <button
                key={id}
                onClick={() => setActiveTab(id)}
                className={`
                  flex items-center gap-2 px-4 py-3 border-b-2 transition-all
                  ${activeTab === id
                    ? 'border-blue-500 text-blue-400'
                    : 'border-transparent text-gray-400 hover:text-gray-200 hover:border-gray-600'
                  }
                `}
              >
                <Icon size={18} />
                <span className="font-medium">{label}</span>
              </button>
            ))}
          </nav>
        </div>

        <div className="bg-gray-800 rounded-lg p-6 shadow-xl">
          {activeTab === 'websocket' && <WebSocketTab />}
          {activeTab === 'metrics' && <MetricsTab />}
          {activeTab === 'container' && <ContainerTab />}
        </div>
      </div>
    </div>
  );
}

export default App;