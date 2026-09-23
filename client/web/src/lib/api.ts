// Typed calls to the client's /api/v1 endpoints

export interface HostSummary {
  name: string;
  up: boolean;
  lastSeen: number;
  time: number;
  os: string;
  uptimeSeconds: number;
  cpuPct: number;
  memUsedPct: number;
  swapUsedPct: number;
  diskUsedPct: number;
  rxBps: number;
  txBps: number;
  activeAlerts: number;
  // 2 critical, 1 warning, 0 no open alerts
  worstSeverity: number;
}

export interface SeriesData {
  label: string;
  // [unix seconds, value]
  points: [number, number][];
}

export interface SeriesResponse {
  metric: string;
  source: string;
  stepSeconds: number;
  series: SeriesData[];
}

export interface AlertRecord {
  id: number;
  host: string;
  rule: string;
  metric: string;
  target: string;
  // 1 warning, 2 critical
  severity: number;
  value: number;
  startedAt: number;
  updatedAt: number;
  // 0 while open
  resolvedAt: number;
}

// The agent's snapshot keeps the Go field names
export interface Process {
  Pid: number;
  Name: string;
  ExecPath: string;
  User: string;
  CPUUsage: number;
  MemUsage: number;
  State: string;
  Threads: number;
}

export interface Processes {
  CPU: Process[] | null;
  Memory: Process[] | null;
}

export interface Snapshot {
  UnixTime: string;
  System: {
    HostName: string;
    OS: string;
    Kernel: string;
    UpTimeSeconds: number;
    LastBootDate: string;
    TimeZone: string;
    LoggedInUsers: { Username: string; RemoteHost: string; LoggedInTime: string }[] | null;
  };
  Memory: { PercentageUsed: number; Used: number; Available: number; Total: number; Unit: string };
  Swap: { PercentageUsed: number; Used: number; Total: number; Unit: string };
  ProcUsage: {
    LoadAvg: number;
    Load1: number;
    Load5: number;
    Load15: number;
    Model: string;
    NoOfCores: number;
    PhysicalCores: number;
    Sockets: number;
  };
  Disk: {
    FileSystem: string;
    Type: string;
    MountedOn: string;
    Usage: { Size: number; Used: number; Available: number; Usage: string };
    Inodes: { Usage: string };
  }[] | null;
  Networks: {
    Interface: string;
    Ip: string;
    Ipv6: string;
    MacAddress: string;
    Usage: { RxBytes: number; TxBytes: number; State: string };
    Rates?: { RxBytesPerSec: number; TxBytesPerSec: number };
  }[] | null;
  Services: { Name: string; Running: boolean }[] | null;
  Processes: Processes;
}

export interface HostDetail {
  host: string;
  time: number;
  lastSeen: number;
  up: boolean;
  snapshot: Snapshot;
}

export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
  }
}

async function get<T>(path: string, params: Record<string, string | number | boolean | undefined> = {}): Promise<T> {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '' && value !== false) {
      query.set(key, value === true ? '1' : String(value));
    }
  }
  const url = query.size > 0 ? `${path}?${query}` : path;
  const response = await fetch(url);
  if (!response.ok) {
    let message = response.statusText;
    try {
      message = (await response.json()).error ?? message;
    } catch {
      // not json, keep the status text
    }
    throw new ApiError(response.status, message);
  }
  return response.json();
}

const host = (name: string) => `/api/v1/hosts/${encodeURIComponent(name)}`;

export const api = {
  config: () => get<{ refreshSeconds: number }>('/api/v1/config'),
  fleet: () => get<{ hosts: HostSummary[] }>('/api/v1/fleet'),
  host: (name: string) => get<HostDetail>(host(name)),
  series: (name: string, metric: string, from: number, to: number, options: { label?: string; maxPoints?: number; max?: boolean } = {}) =>
    get<SeriesResponse>(`${host(name)}/series`, { metric, from, to, ...options }),
  processes: (name: string, at?: number) => get<{ time: number; processes: Processes }>(`${host(name)}/processes`, { at }),
  customMetrics: (name: string) => get<{ names: string[] }>(`${host(name)}/custom-metrics`),
  alerts: (filter: { host?: string; open?: boolean; from?: number; to?: number } = {}) =>
    get<{ alerts: AlertRecord[] }>('/api/v1/alerts', filter),
};
