import { atom } from 'jotai';

// Tab state
export const activeTabAtom = atom<'websocket' | 'metrics' | 'container'>('websocket');

// Server selection
export const selectedServerAtom = atom<'node' | 'go'>('node');

// WebSocket state
export const wsConnectionAtom = atom<WebSocket | null>(null);
export const wsStatusAtom = atom<'disconnected' | 'connecting' | 'connected' | 'reconnecting'>('disconnected');
export const wsMessagesAtom = atom<Array<{ time: string; type: string; data: any }>>([]);
export const reconnectAtom = atom<{
  timeoutId: NodeJS.Timeout | null;
  attempt: number;
}>({
  timeoutId: null,
  attempt: 0,
});

// Metrics state
export const metricsDataAtom = atom<{
  connections: number[];
  memory: number[];
  cpu: number[];
  goroutines: number[];
  messages: number[];
  timestamps: string[];
}>({
  connections: [],
  memory: [],
  cpu: [],
  goroutines: [],
  messages: [],
  timestamps: []
});

// Container specs state
export const containerSpecsAtom = atom<{
  node: {
    cpu: string;
    memory: string;
    pids: number;
    diskIO: number;
    status: string;
  };
  go: {
    cpu: string;
    memory: string;
    pids: number;
    diskIO: number;
    status: string;
  };
}>({
  node: {
    cpu: '2.0 cores',
    memory: '512MB',
    pids: 200,
    diskIO: 500,
    status: 'healthy'
  },
  go: {
    cpu: '2.0 cores',
    memory: '512MB',
    pids: 200,
    diskIO: 500,
    status: 'healthy'
  }
});

// Live container stats
export const liveContainerStatsAtom = atom<{
  node: {
    cpuUsage: number;
    memoryUsage: number;
    memoryPercent: number;
    networkIO: string;
  };
  go: {
    cpuUsage: number;
    memoryUsage: number;
    memoryPercent: number;
    networkIO: string;
  };
}>({
  node: {
    cpuUsage: 0,
    memoryUsage: 0,
    memoryPercent: 0,
    networkIO: '0B / 0B'
  },
  go: {
    cpuUsage: 0,
    memoryUsage: 0,
    memoryPercent: 0,
    networkIO: '0B / 0B'
  }
});