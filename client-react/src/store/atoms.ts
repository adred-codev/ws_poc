import { atom } from 'jotai';

// The active tab in the main UI
export const activeTabAtom = atom<'websocket' | 'metrics' | 'container'>('websocket');

// The server currently being viewed in the UI
export const selectedServerAtom = atom<'node' | 'go'>('node');

// --- WebSocket State ---

type WsStatus = 'disconnected' | 'connecting' | 'connected' | 'reconnecting';
type WsMessage = { time: string; type: string; data: any };
type ReconnectState = { timeoutId: NodeJS.Timeout | null; attempt: number };

export type ServerState = {
  ws: WebSocket | null;
  status: WsStatus;
  messages: WsMessage[];
  reconnect: ReconnectState;
  isIntentionalDisconnect: boolean;
};

export const webSocketStateAtom = atom<{
  [key in 'node' | 'go']: ServerState;
}>({
  node: {
    ws: null,
    status: 'disconnected',
    messages: [],
    reconnect: { timeoutId: null, attempt: 0 },
    isIntentionalDisconnect: false,
  },
  go: {
    ws: null,
    status: 'disconnected',
    messages: [],
    reconnect: { timeoutId: null, attempt: 0 },
    isIntentionalDisconnect: false,
  },
});

// --- Metrics State ---

export type MetricsData = {
  labels: string[];
  connections: number[];
  memory: number[];
  cpu: number[];
};

export const metricsDataAtom = atom<{
  [key in 'node' | 'go']: MetricsData;
}>({
  node: { labels: [], connections: [], memory: [], cpu: [] },
  go: { labels: [], connections: [], memory: [], cpu: [] },
});

// --- Container State ---

export const containerSpecsAtom = atom<{
  [key in 'node' | 'go']: { cpu: string; memory: string; pids: number; diskIO: number; status: string; };
}>({
  node: { cpu: '2.0 cores', memory: '512MB', pids: 200, diskIO: 500, status: 'healthy' },
  go: { cpu: '2.0 cores', memory: '512MB', pids: 200, diskIO: 500, status: 'healthy' },
});

export const liveContainerStatsAtom = atom<{
  [key in 'node' | 'go']: { cpuUsage: number; memoryUsage: number; memoryPercent: number; networkIO: string; };
}>({
  node: { cpuUsage: 0, memoryUsage: 0, memoryPercent: 0, networkIO: '0B / 0B' },
  go: { cpuUsage: 0, memoryUsage: 0, memoryPercent: 0, networkIO: '0B / 0B' },
});


// --- DERIVED ATOMS FOR EASY ACCESS/UPDATES ---

// --- WebSocket Derived Atoms ---

export const currentServerStateAtom = atom((get) => {
  const selected = get(selectedServerAtom);
  return get(webSocketStateAtom)[selected];
});

export const currentMessagesAtom = atom((get) => {
  const selected = get(selectedServerAtom);
  return get(webSocketStateAtom)[selected].messages;
});

export const updateServerStateAtom = atom(
  null,
  (get, set, { server, partialState }: { server: 'node' | 'go', partialState: Partial<ServerState> }) => {
    const state = get(webSocketStateAtom);
    set(webSocketStateAtom, {
      ...state,
      [server]: { ...state[server], ...partialState },
    });
  }
);

export const addMessageAtom = atom(
  null,
  (get, set, { server, type, data }: { server: 'node' | 'go', type: string, data: any }) => {
    const state = get(webSocketStateAtom);
    const newMessage = { time: new Date().toLocaleTimeString(), type, data };
    set(webSocketStateAtom, {
      ...state,
      [server]: {
        ...state[server],
        messages: [...state[server].messages, newMessage],
      },
    });
  }
);

// --- Metrics Derived Atoms ---

export const currentMetricsDataAtom = atom((get) => {
  const selected = get(selectedServerAtom);
  return get(metricsDataAtom)[selected];
});

export const addMetricsDataAtom = atom(
  null,
  (get, set, { server, metrics }: { server: 'node' | 'go', metrics: { connections: number; memory: number; cpu: number } }) => {
    const state = get(metricsDataAtom);
    const now = new Date();
    const newLabel = `${now.getHours()}:${now.getMinutes()}:${now.getSeconds()}`;

    const newState = {
      ...state,
      [server]: {
        labels: [...state[server].labels.slice(-29), newLabel],
        connections: [...state[server].connections.slice(-29), metrics.connections],
        memory: [...state[server].memory.slice(-29), metrics.memory],
        cpu: [...state[server].cpu.slice(-29), metrics.cpu],
      },
    };
    set(metricsDataAtom, newState);
  }
);

// --- Container Derived Atoms ---

export const currentContainerSpecsAtom = atom((get) => {
  const selected = get(selectedServerAtom);
  return get(containerSpecsAtom)[selected];
});

export const currentLiveContainerStatsAtom = atom((get) => {
  const selected = get(selectedServerAtom);
  return get(liveContainerStatsAtom)[selected];
});
