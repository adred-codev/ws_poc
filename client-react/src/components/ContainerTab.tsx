import { useAtom } from 'jotai';
import { useEffect } from 'react';
import { Cpu, HardDrive, Activity, Network, CheckCircle, XCircle } from 'lucide-react';
import { containerSpecsAtom, liveContainerStatsAtom } from '../store/atoms';

function ContainerTab() {
  const [specs] = useAtom(containerSpecsAtom);
  const [liveStats, setLiveStats] = useAtom(liveContainerStatsAtom);

  useEffect(() => {
    // Fetch live container stats
    const fetchStats = async () => {
      try {
        // This would call docker stats API endpoint
        // For now using mock data
        setLiveStats({
          node: {
            cpuUsage: Math.random() * 100,
            memoryUsage: 256 + Math.random() * 256,
            memoryPercent: 50 + Math.random() * 50,
            networkIO: `${Math.floor(Math.random() * 1000)}KB / ${Math.floor(Math.random() * 1000)}KB`
          },
          go: {
            cpuUsage: Math.random() * 100,
            memoryUsage: 256 + Math.random() * 256,
            memoryPercent: 50 + Math.random() * 50,
            networkIO: `${Math.floor(Math.random() * 1000)}KB / ${Math.floor(Math.random() * 1000)}KB`
          }
        });
      } catch (error) {
        console.error('Failed to fetch container stats:', error);
      }
    };

    fetchStats();
    const interval = setInterval(fetchStats, 5000);
    return () => clearInterval(interval);
  }, [setLiveStats]);

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold mb-4">Container Resource Specifications</h2>
        <p className="text-gray-400">
          Both WebSocket servers are configured with identical resource constraints to ensure fair performance comparison.
        </p>
      </div>

      <div className="grid grid-cols-2 gap-6">
        <ServerCard
          title="Node.js WebSocket Server"
          specs={specs.node}
          liveStats={liveStats.node}
          color="green"
        />
        <ServerCard
          title="Go WebSocket Server"
          specs={specs.go}
          liveStats={liveStats.go}
          color="blue"
        />
      </div>

      <div className="bg-gray-700 rounded-lg p-6">
        <h3 className="text-lg font-semibold mb-4">Shared Configuration</h3>
        <div className="grid grid-cols-3 gap-4 text-sm">
          <div>
            <div className="text-gray-400 mb-1">Network</div>
            <div className="font-medium">odin-network (Bridge)</div>
          </div>
          <div>
            <div className="text-gray-400 mb-1">Restart Policy</div>
            <div className="font-medium">unless-stopped</div>
          </div>
          <div>
            <div className="text-gray-400 mb-1">Health Check</div>
            <div className="font-medium">Every 30s</div>
          </div>
        </div>
      </div>

      <div className="bg-gray-700 rounded-lg p-6">
        <h3 className="text-lg font-semibold mb-4">Resource Limits Explained</h3>
        <div className="space-y-3 text-sm">
          <div className="flex items-start gap-3">
            <Cpu className="text-blue-400 mt-0.5" size={16} />
            <div>
              <div className="font-medium">CPU: 2.0 cores max, 1.0 core guaranteed</div>
              <div className="text-gray-400">Each server can burst up to 2 CPU cores but is guaranteed at least 1 core</div>
            </div>
          </div>
          <div className="flex items-start gap-3">
            <HardDrive className="text-green-400 mt-0.5" size={16} />
            <div>
              <div className="font-medium">Memory: 512MB max, 256MB reserved</div>
              <div className="text-gray-400">Hard limit at 512MB with 256MB always reserved</div>
            </div>
          </div>
          <div className="flex items-start gap-3">
            <Activity className="text-yellow-400 mt-0.5" size={16} />
            <div>
              <div className="font-medium">Process limit: 200 PIDs</div>
              <div className="text-gray-400">Maximum number of processes/threads per container</div>
            </div>
          </div>
          <div className="flex items-start gap-3">
            <Network className="text-purple-400 mt-0.5" size={16} />
            <div>
              <div className="font-medium">Disk I/O weight: 500</div>
              <div className="text-gray-400">Relative priority for disk operations (100-1000 scale)</div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

function ServerCard({ title, specs, liveStats, color }: {
  title: string;
  specs: any;
  liveStats: any;
  color: string;
}) {
  const colorClasses = {
    green: 'border-green-500 bg-green-900/10',
    blue: 'border-blue-500 bg-blue-900/10'
  };

  return (
    <div className={`border rounded-lg p-6 ${colorClasses[color as keyof typeof colorClasses]}`}>
      <div className="flex items-center justify-between mb-4">
        <h3 className="text-lg font-semibold">{title}</h3>
        <div className="flex items-center gap-1">
          {specs.status === 'healthy' ? (
            <>
              <CheckCircle className="text-green-400" size={20} />
              <span className="text-green-400 text-sm">Healthy</span>
            </>
          ) : (
            <>
              <XCircle className="text-red-400" size={20} />
              <span className="text-red-400 text-sm">Unhealthy</span>
            </>
          )}
        </div>
      </div>

      <div className="space-y-4">
        <div>
          <div className="text-gray-400 text-sm mb-2">Specifications</div>
          <div className="grid grid-cols-2 gap-3 text-sm">
            <div>
              <div className="text-gray-500">CPU</div>
              <div className="font-medium">{specs.cpu}</div>
            </div>
            <div>
              <div className="text-gray-500">Memory</div>
              <div className="font-medium">{specs.memory}</div>
            </div>
            <div>
              <div className="text-gray-500">PIDs Limit</div>
              <div className="font-medium">{specs.pids}</div>
            </div>
            <div>
              <div className="text-gray-500">Disk I/O</div>
              <div className="font-medium">{specs.diskIO}</div>
            </div>
          </div>
        </div>

        <div>
          <div className="text-gray-400 text-sm mb-2">Live Usage</div>
          <div className="space-y-2">
            <UsageBar
              label="CPU"
              value={liveStats.cpuUsage}
              max={100}
              unit="%"
              color="blue"
            />
            <UsageBar
              label="Memory"
              value={liveStats.memoryPercent}
              max={100}
              unit="%"
              detail={`${liveStats.memoryUsage.toFixed(0)}MB / 512MB`}
              color="green"
            />
          </div>
          <div className="mt-2 text-sm">
            <div className="text-gray-500">Network I/O</div>
            <div className="font-medium">{liveStats.networkIO}</div>
          </div>
        </div>
      </div>
    </div>
  );
}

function UsageBar({ label, value, max, unit, detail, color }: {
  label: string;
  value: number;
  max: number;
  unit: string;
  detail?: string;
  color: string;
}) {
  const percentage = (value / max) * 100;
  const colorClasses = {
    blue: 'bg-blue-500',
    green: 'bg-green-500'
  };

  return (
    <div>
      <div className="flex justify-between text-xs mb-1">
        <span className="text-gray-400">{label}</span>
        <span className="text-gray-300">
          {value.toFixed(1)}{unit} {detail && <span className="text-gray-500">({detail})</span>}
        </span>
      </div>
      <div className="h-2 bg-gray-600 rounded-full overflow-hidden">
        <div
          className={`h-full ${colorClasses[color as keyof typeof colorClasses]} transition-all duration-500`}
          style={{ width: `${percentage}%` }}
        />
      </div>
    </div>
  );
}

export default ContainerTab;