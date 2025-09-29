import { useAtom, useSetAtom } from 'jotai';
import { useEffect } from 'react';
import { LineChart, Line, AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts';
import { Activity, Cpu, HardDrive } from 'lucide-react';
import { selectedServerAtom, currentMetricsDataAtom, addMetricsDataAtom } from '../store/atoms';

function MetricsTab() {
  const [selectedServer, setSelectedServer] = useAtom(selectedServerAtom);
  const [metricsData] = useAtom(currentMetricsDataAtom);
  const addMetrics = useSetAtom(addMetricsDataAtom);

  useEffect(() => {
    const fetchMetricsForServer = async (server: 'node' | 'go') => {
      try {
        const endpoint = server === 'node'
          ? 'http://localhost:3001/metrics'
          : 'http://localhost:3002/stats';

        const response = await fetch(endpoint);
        if (!response.ok) {
          throw new Error(`Failed to fetch metrics from ${server}: ${response.statusText}`);
        }
        const data = await response.json();

        // Normalize data to a common format
        let normalizedMetrics;
        
        if (server === 'node') {
          // Node.js metrics - only has connection count, no system metrics
          // We'll need to add simulated values or extend the server
          normalizedMetrics = {
            connections: data.connectionCount || 0,
            // Since Node.js doesn't expose memory/CPU, we simulate realistic values
            // In production, you'd add process.memoryUsage() and process.cpuUsage() to the server
            memory: 100 + (Math.sin(Date.now() / 10000) + 1) * 50, // Oscillating 100-200MB
            cpu: 5 + (Math.sin(Date.now() / 5000) + 1) * 7.5, // Oscillating 5-20%
          };
        } else {
          // Go metrics - has real system stats
          normalizedMetrics = {
            connections: data.connections?.active || 0,
            memory: (data.system?.memory?.heap_alloc || 0) / 1024 / 1024, // Convert bytes to MB
            // Go doesn't directly provide CPU %, but we can derive activity from goroutines
            cpu: Math.min(100, (data.system?.goroutines || 0) * 0.5), // Estimate based on goroutines
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
    const interval = setInterval(fetchAllMetrics, 5000);
    return () => clearInterval(interval);
  }, [addMetrics]);

  const chartData = metricsData.labels.map((label, idx) => ({
    time: label,
    connections: metricsData.connections[idx] || 0,
    memory: metricsData.memory[idx] || 0,
    cpu: metricsData.cpu[idx] || 0,
  }));

  const currentMetrics = chartData[chartData.length - 1] || {
    connections: 0,
    memory: 0,
    cpu: 0,
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">Real-time Performance Metrics</h2>
          <p className="text-xs mt-1 text-gray-400">
            <span className="text-green-400">● Real data:</span> Connections (both), Memory (Go only) | 
            <span className="text-yellow-400 ml-2">● Simulated:</span> CPU (both), Memory (Node.js)
          </p>
        </div>
        <select
          value={selectedServer}
          onChange={(e) => setSelectedServer(e.target.value as 'node' | 'go')}
          className="bg-gray-700 text-gray-100 px-4 py-2 rounded-md border border-gray-600 focus:border-blue-500 focus:outline-none"
        >
          <option value="node">Node.js Server</option>
          <option value="go">Go Server</option>
        </select>
      </div>

      <div className="grid grid-cols-3 gap-4">
        <MetricCard
          icon={Activity}
          label="Connections"
          value={currentMetrics.connections}
          color="blue"
          subtitle="Real-time"
        />
        <MetricCard
          icon={HardDrive}
          label="Memory (MB)"
          value={currentMetrics.memory.toFixed(2)}
          color="green"
          subtitle={selectedServer === 'go' ? 'Real-time' : 'Simulated'}
        />
        <MetricCard
          icon={Cpu}
          label="CPU %"
          value={currentMetrics.cpu.toFixed(2)}
          color="yellow"
          subtitle="Estimated"
        />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
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