import { useAtom, useSetAtom } from 'jotai';
import { useEffect } from 'react';
import { LineChart, Line, AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts';
import { Activity, Cpu, HardDrive, Server } from 'lucide-react';
import { metricsDataAtom, addMetricsDataAtom } from '../store/atoms';

// Component for individual server metrics pane
const ServerMetricsPane = ({ server, title }: { server: 'node' | 'go'; title: string }) => {
  const [metricsData] = useAtom(metricsDataAtom);
  const serverMetrics = metricsData[server];

  const chartData = serverMetrics.labels.map((label, idx) => ({
    time: label,
    connections: serverMetrics.connections[idx] || 0,
    memory: serverMetrics.memory[idx] || 0,
    cpu: serverMetrics.cpu[idx] || 0,
  }));

  const currentMetrics = chartData[chartData.length - 1] || {
    connections: 0,
    memory: 0,
    cpu: 0,
  };

  const serverColor = server === 'node' ? 'green' : 'blue';
  const chartColors = {
    node: {
      connections: '#10B981',
      memory: '#34D399',
      cpu: '#6EE7B7'
    },
    go: {
      connections: '#3B82F6',
      memory: '#60A5FA',
      cpu: '#93BBFC'
    }
  };

  return (
    <div className="bg-gray-800 rounded-lg p-6 space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-gray-700 pb-4">
        <div className="flex items-center gap-3">
          <Server size={20} className={server === 'node' ? 'text-green-400' : 'text-blue-400'} />
          <h3 className="text-lg font-semibold text-gray-200">{title}</h3>
          <span className={`px-2 py-1 rounded text-xs ${
            server === 'node' ? 'bg-green-900/50 text-green-400' : 'bg-blue-900/50 text-blue-400'
          }`}>
            {server === 'node' ? 'Port 3001' : 'Port 3004'}
          </span>
        </div>
        <div className="text-xs text-gray-400">
          {server === 'node' ? '● Real-time CPU/Memory' : '● Real-time CPU/Memory'}
        </div>
      </div>

      {/* Metric Cards */}
      <div className="grid grid-cols-3 gap-4">
        <MetricCard
          icon={Activity}
          label="Connections"
          value={currentMetrics.connections}
          color={serverColor}
          subtitle="Real-time"
        />
        <MetricCard
          icon={HardDrive}
          label="Memory (MB)"
          value={currentMetrics.memory.toFixed(2)}
          color={serverColor}
          subtitle="Real-time"
        />
        <MetricCard
          icon={Cpu}
          label="CPU %"
          value={currentMetrics.cpu.toFixed(2)}
          color={serverColor}
          subtitle="Real-time"
        />
      </div>

      {/* Charts */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <ChartCard title="Active Connections">
          <ResponsiveContainer width="100%" height={200}>
            <AreaChart data={chartData}>
              <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
              <XAxis dataKey="time" stroke="#9CA3AF" fontSize={10} />
              <YAxis stroke="#9CA3AF" fontSize={10} />
              <Tooltip
                contentStyle={{ backgroundColor: '#1F2937', border: '1px solid #374151' }}
                labelStyle={{ color: '#9CA3AF' }}
              />
              <Area
                type="monotone"
                dataKey="connections"
                stroke={chartColors[server].connections}
                fill={chartColors[server].connections}
                fillOpacity={0.3}
              />
            </AreaChart>
          </ResponsiveContainer>
        </ChartCard>

        <ChartCard title="Memory Usage (MB)">
          <ResponsiveContainer width="100%" height={200}>
            <LineChart data={chartData}>
              <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
              <XAxis dataKey="time" stroke="#9CA3AF" fontSize={10} />
              <YAxis stroke="#9CA3AF" fontSize={10} />
              <Tooltip
                contentStyle={{ backgroundColor: '#1F2937', border: '1px solid #374151' }}
                labelStyle={{ color: '#9CA3AF' }}
              />
              <Line
                type="monotone"
                dataKey="memory"
                stroke={chartColors[server].memory}
                strokeWidth={2}
                dot={false}
              />
            </LineChart>
          </ResponsiveContainer>
        </ChartCard>

        <ChartCard title="CPU Usage (%)">
          <ResponsiveContainer width="100%" height={200}>
            <AreaChart data={chartData}>
              <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
              <XAxis dataKey="time" stroke="#9CA3AF" fontSize={10} />
              <YAxis stroke="#9CA3AF" fontSize={10} />
              <Tooltip
                contentStyle={{ backgroundColor: '#1F2937', border: '1px solid #374151' }}
                labelStyle={{ color: '#9CA3AF' }}
              />
              <Area
                type="monotone"
                dataKey="cpu"
                stroke={chartColors[server].cpu}
                fill={chartColors[server].cpu}
                fillOpacity={0.3}
              />
            </AreaChart>
          </ResponsiveContainer>
        </ChartCard>
      </div>
    </div>
  );
};

function MetricsTab() {
  const [metricsData] = useAtom(metricsDataAtom);
  const addMetrics = useSetAtom(addMetricsDataAtom);

  useEffect(() => {
    const fetchMetricsForServer = async (server: 'node' | 'go') => {
      try {
        const endpoint = server === 'node'
          ? 'http://localhost:3001/metrics'
          : 'http://localhost:3004/stats';

        const response = await fetch(endpoint);
        if (!response.ok) {
          throw new Error(`Failed to fetch metrics from ${server}: ${response.statusText}`);
        }
        const data = await response.json();

        // Normalize data to a common format
        let normalizedMetrics;

        if (server === 'node') {
          // Node.js enhanced metrics - now has accurate CPU and memory stats
          normalizedMetrics = {
            connections: data.connectionCount || 0,
            memory: data.memory || 0, // Accurate memory from systeminformation
            cpu: data.cpu || 0, // Accurate CPU from systeminformation
          };
        } else {
          // Go stats - now includes CPU and memory
          normalizedMetrics = {
            connections: data.currentConnections || 0,
            memory: data.memoryMB || 0,
            cpu: data.cpuPercent || 0,
          };
        }

        addMetrics({ server, metrics: normalizedMetrics });

      } catch (error) {
        console.error(`Failed to fetch metrics for ${server}:`, error);
        // Add default values on error to keep the charts updating
        addMetrics({
          server,
          metrics: {
            connections: 0,
            memory: 0,
            cpu: 0
          }
        });
      }
    };

    const fetchAllMetrics = () => {
      fetchMetricsForServer('node');
      fetchMetricsForServer('go');
    };

    fetchAllMetrics();
    const interval = setInterval(fetchAllMetrics, 3000);
    return () => clearInterval(interval);
  }, [addMetrics]);

  // Calculate combined stats
  const nodeData = metricsData.node;
  const goData = metricsData.go;

  const totalConnections =
    (nodeData.connections[nodeData.connections.length - 1] || 0) +
    (goData.connections[goData.connections.length - 1] || 0);

  const totalMemory =
    (nodeData.memory[nodeData.memory.length - 1] || 0) +
    (goData.memory[goData.memory.length - 1] || 0);

  const avgCpu =
    ((nodeData.cpu[nodeData.cpu.length - 1] || 0) +
    (goData.cpu[goData.cpu.length - 1] || 0)) / 2;

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">Real-time Performance Metrics</h2>
          <p className="text-xs mt-1 text-gray-400">
            Monitoring both Node.js and Go WebSocket servers
          </p>
        </div>
        <div className="flex gap-4 text-sm">
          <div className="bg-gray-700 px-3 py-1 rounded">
            Total Connections: <span className="text-purple-400 font-bold">{totalConnections}</span>
          </div>
          <div className="bg-gray-700 px-3 py-1 rounded">
            Total Memory: <span className="text-purple-400 font-bold">{totalMemory.toFixed(1)} MB</span>
          </div>
          <div className="bg-gray-700 px-3 py-1 rounded">
            Avg CPU: <span className="text-purple-400 font-bold">{avgCpu.toFixed(1)}%</span>
          </div>
        </div>
      </div>

      {/* Server Metrics Panes - Stacked Vertically */}
      <div className="space-y-6">
        {/* Node.js Metrics */}
        <ServerMetricsPane server="node" title="Node.js Server Metrics" />

        {/* Go Metrics */}
        <ServerMetricsPane server="go" title="Go Server Metrics" />
      </div>
    </div>
  );
}

function MetricCard({ icon: Icon, label, value, color, subtitle }: {
  icon: any;
  label: string;
  value: string | number;
  color: string;
  subtitle?: string;
}) {
  const colorClasses: { [key: string]: string } = {
    blue: 'text-blue-400 bg-blue-900/20',
    green: 'text-green-400 bg-green-900/20',
    yellow: 'text-yellow-400 bg-yellow-900/20',
    purple: 'text-purple-400 bg-purple-900/20'
  };

  return (
    <div className="bg-gray-800 rounded-lg p-4 shadow-lg">
      <div className="flex items-center justify-between mb-2">
        <div>
          <span className="text-gray-400 text-sm font-medium">{label}</span>
          {subtitle && <span className="text-xs text-gray-500 block">{subtitle}</span>}
        </div>
        <div className={`p-2 rounded-lg ${colorClasses[color]}`}>
          <Icon size={16} />
        </div>
      </div>
      <div className="text-3xl font-bold text-gray-50">{value}</div>
    </div>
  );
}

function ChartCard({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="bg-gray-800 rounded-lg p-4 shadow-lg">
      <h3 className="text-lg font-semibold text-gray-200 mb-4">{title}</h3>
      {children}
    </div>
  );
}

export default MetricsTab;