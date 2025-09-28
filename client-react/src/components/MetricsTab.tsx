import { useAtom } from 'jotai';
import { useEffect } from 'react';
import { LineChart, Line, AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts';
import { Activity, Cpu, HardDrive, GitBranch } from 'lucide-react';
import { selectedServerAtom, metricsDataAtom } from '../store/atoms';

function MetricsTab() {
  const [selectedServer, setSelectedServer] = useAtom(selectedServerAtom);
  const [metricsData, setMetricsData] = useAtom(metricsDataAtom);

  useEffect(() => {
    const fetchMetrics = async () => {
      try {
        const endpoint = selectedServer === 'node'
          ? '/api/node/stats'
          : '/api/go/stats';

        const response = await fetch(endpoint);
        const data = await response.json();

        setMetricsData((prev) => {
          const now = new Date().toLocaleTimeString();
          return {
            connections: [...prev.connections.slice(-19), data.connections?.active || 0],
            memory: [...prev.memory.slice(-19), (data.system?.memory?.alloc || 0) / 1024 / 1024],
            cpu: [...prev.cpu.slice(-19), data.system?.cpu || 0],
            goroutines: [...prev.goroutines.slice(-19), data.system?.goroutines || 0],
            messages: [...prev.messages.slice(-19), data.nats?.InMsgs || 0],
            timestamps: [...prev.timestamps.slice(-19), now]
          };
        });
      } catch (error) {
        console.error('Failed to fetch metrics:', error);
      }
    };

    fetchMetrics();
    const interval = setInterval(fetchMetrics, 5000);
    return () => clearInterval(interval);
  }, [selectedServer, setMetricsData]);

  const chartData = metricsData.timestamps.map((time, idx) => ({
    time,
    connections: metricsData.connections[idx] || 0,
    memory: metricsData.memory[idx] || 0,
    cpu: metricsData.cpu[idx] || 0,
    goroutines: metricsData.goroutines[idx] || 0,
    messages: metricsData.messages[idx] || 0
  }));

  const currentMetrics = chartData[chartData.length - 1] || {
    connections: 0,
    memory: 0,
    cpu: 0,
    goroutines: 0,
    messages: 0
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Real-time Performance Metrics</h2>
        <select
          value={selectedServer}
          onChange={(e) => setSelectedServer(e.target.value as 'node' | 'go')}
          className="bg-gray-700 text-gray-100 px-4 py-2 rounded-md border border-gray-600 focus:border-blue-500 focus:outline-none"
        >
          <option value="node">Node.js Server</option>
          <option value="go">Go Server</option>
        </select>
      </div>

      <div className="grid grid-cols-4 gap-4">
        <MetricCard
          icon={Activity}
          label="Connections"
          value={currentMetrics.connections}
          color="blue"
        />
        <MetricCard
          icon={HardDrive}
          label="Memory (MB)"
          value={currentMetrics.memory.toFixed(1)}
          color="green"
        />
        <MetricCard
          icon={Cpu}
          label="CPU %"
          value={currentMetrics.cpu.toFixed(1)}
          color="yellow"
        />
        <MetricCard
          icon={GitBranch}
          label={selectedServer === 'go' ? 'Goroutines' : 'Event Loop'}
          value={currentMetrics.goroutines}
          color="purple"
        />
      </div>

      <div className="grid grid-cols-2 gap-6">
        <ChartCard title="Active Connections">
          <ResponsiveContainer width="100%" height={250}>
            <AreaChart data={chartData}>
              <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
              <XAxis dataKey="time" stroke="#9CA3AF" fontSize={12} />
              <YAxis stroke="#9CA3AF" fontSize={12} />
              <Tooltip
                contentStyle={{ backgroundColor: '#1F2937', border: '1px solid #374151' }}
                labelStyle={{ color: '#9CA3AF' }}
              />
              <Area type="monotone" dataKey="connections" stroke="#3B82F6" fill="#3B82F6" fillOpacity={0.3} />
            </AreaChart>
          </ResponsiveContainer>
        </ChartCard>

        <ChartCard title="Memory Usage (MB)">
          <ResponsiveContainer width="100%" height={250}>
            <LineChart data={chartData}>
              <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
              <XAxis dataKey="time" stroke="#9CA3AF" fontSize={12} />
              <YAxis stroke="#9CA3AF" fontSize={12} />
              <Tooltip
                contentStyle={{ backgroundColor: '#1F2937', border: '1px solid #374151' }}
                labelStyle={{ color: '#9CA3AF' }}
              />
              <Line type="monotone" dataKey="memory" stroke="#10B981" strokeWidth={2} dot={false} />
            </LineChart>
          </ResponsiveContainer>
        </ChartCard>

        <ChartCard title="CPU Usage (%)">
          <ResponsiveContainer width="100%" height={250}>
            <AreaChart data={chartData}>
              <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
              <XAxis dataKey="time" stroke="#9CA3AF" fontSize={12} />
              <YAxis stroke="#9CA3AF" fontSize={12} />
              <Tooltip
                contentStyle={{ backgroundColor: '#1F2937', border: '1px solid #374151' }}
                labelStyle={{ color: '#9CA3AF' }}
              />
              <Area type="monotone" dataKey="cpu" stroke="#F59E0B" fill="#F59E0B" fillOpacity={0.3} />
            </AreaChart>
          </ResponsiveContainer>
        </ChartCard>

        <ChartCard title={selectedServer === 'go' ? 'Goroutines' : 'Event Loop Tasks'}>
          <ResponsiveContainer width="100%" height={250}>
            <LineChart data={chartData}>
              <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
              <XAxis dataKey="time" stroke="#9CA3AF" fontSize={12} />
              <YAxis stroke="#9CA3AF" fontSize={12} />
              <Tooltip
                contentStyle={{ backgroundColor: '#1F2937', border: '1px solid #374151' }}
                labelStyle={{ color: '#9CA3AF' }}
              />
              <Line type="monotone" dataKey="goroutines" stroke="#A855F7" strokeWidth={2} dot={false} />
            </LineChart>
          </ResponsiveContainer>
        </ChartCard>
      </div>
    </div>
  );
}

function MetricCard({ icon: Icon, label, value, color }: {
  icon: any;
  label: string;
  value: string | number;
  color: string;
}) {
  const colorClasses = {
    blue: 'text-blue-400 bg-blue-900/20',
    green: 'text-green-400 bg-green-900/20',
    yellow: 'text-yellow-400 bg-yellow-900/20',
    purple: 'text-purple-400 bg-purple-900/20'
  };

  return (
    <div className="bg-gray-700 rounded-lg p-4">
      <div className="flex items-center justify-between mb-2">
        <span className="text-gray-400 text-sm">{label}</span>
        <div className={`p-2 rounded-lg ${colorClasses[color as keyof typeof colorClasses]}`}>
          <Icon size={16} />
        </div>
      </div>
      <div className="text-2xl font-bold">{value}</div>
    </div>
  );
}

function ChartCard({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="bg-gray-700 rounded-lg p-4">
      <h3 className="text-sm font-medium text-gray-300 mb-4">{title}</h3>
      {children}
    </div>
  );
}

export default MetricsTab;