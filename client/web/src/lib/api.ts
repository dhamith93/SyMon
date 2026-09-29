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
  // running containers at the latest snapshot
  containers: number;
  // days until the first disk is full, null when none is filling up
  diskFullDays: number | null;
  // empty from agents older than versions
  agentVersion: string;
}

// An endpoint's newest check up to the end of a range, and how it did over it
export interface EndpointStatus {
  name: string;
  url: string;
  method: string;
  time: number;
  ok: boolean;
  // 0 when there was no response
  statusCode: number;
  latencyMs: number;
  error: string;
  checks: number;
  uptimePct: number;
  // over the checks that got a response
  avgLatencyMs: number;
}

// A disk's growth over the last week and when it fills up at that rate
export interface DiskForecast {
  device: string;
  mount: string;
  usedPct: number;
  pctPerDay: number;
  bytesPerDay: number;
  // null when the disk is not filling up, and noForecast then says why:
  // collecting, not_growing, not_steady or over_a_year
  daysToFull: number | null;
  noForecast: string;
  // hours of history behind the forecast
  samples: number;
}

// disks that fill up sooner than this many days are shown as warnings
export const diskFullSoonDays = 30;

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

// One program's share of a time range, with its processes added up
export interface ProcessUsage {
  name: string;
  cpuAvg: number;
  cpuPeak: number;
  memAvg: number;
  memPeak: number;
  // share of snapshots the program was in the top lists
  seenPct: number;
}

export interface Processes {
  CPU: Process[] | null;
  Memory: Process[] | null;
}

export interface Container {
  ID: string;
  ShortID: string;
  Name: string;
  Image: string;
  State: string;
  Runtime: string;
  ComposeProject: string;
  MetadataAvailable: boolean;
  CPU: { CoresUsed: number; PercentOfHost: number; PercentOfLimit: number; Limited: boolean; AllocatedCores: number };
  // bytes
  Memory: { Used: number; Limit: number; Limited: boolean; PercentageUsed: number; OOMKills: number };
  Network: { RxBytes: number; TxBytes: number; SharesHostNetwork: boolean; Accessible: boolean };
  BlockIO: { ReadBytes: number; WriteBytes: number };
  Pids: { Current: number; Max: number; Limited: boolean };
  // missing until the agent has two samples. No traffic rates for host network containers.
  Rates?: { RxBytesPerSec?: number; TxBytesPerSec?: number; ReadBytesPerSec: number; WriteBytesPerSec: number };
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
  Containers?: Container[];
  // missing from agents older than it
  AgentVersion?: string;
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

// called when the API says the session is gone, like after it expired
let onUnauthorized = () => {};

export function setUnauthorizedHandler(handler: () => void) {
  onUnauthorized = handler;
}

async function fail(response: Response): Promise<never> {
  let message = response.statusText;
  try {
    message = (await response.json()).error ?? message;
  } catch {
    // not json, keep the status text
  }
  throw new ApiError(response.status, message);
}

async function post<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  if (!response.ok) return fail(response);
  return response.json();
}

// who is logged in, or without a session whether there are users at all
export type SessionState = { user: string } | { user: null; hasUsers: boolean };

async function session(): Promise<SessionState> {
  const response = await fetch('/api/v1/session');
  if (response.ok) return { user: (await response.json()).user };
  if (response.status !== 401) return fail(response);
  return { user: null, hasUsers: !!(await response.json()).hasUsers };
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
  if (response.status === 401) onUnauthorized();
  if (!response.ok) return fail(response);
  return response.json();
}

const host = (name: string) => `/api/v1/hosts/${encodeURIComponent(name)}`;

export const api = {
  session,
  login: (user: string, password: string) => post<{ user: string }>('/api/v1/login', { user, password }),
  logout: () => post<object>('/api/v1/logout', {}),
  // collectorVersion is empty when the collector cannot say
  config: () => get<{ refreshSeconds: number; version: string; collectorVersion: string }>('/api/v1/config'),
  fleet: () => get<{ hosts: HostSummary[] }>('/api/v1/fleet'),
  host: (name: string) => get<HostDetail>(host(name)),
  series: (name: string, metric: string, from: number, to: number, options: { label?: string; maxPoints?: number; max?: boolean } = {}) =>
    get<SeriesResponse>(`${host(name)}/series`, { metric, from, to, ...options }),
  processes: (name: string, at?: number) => get<{ time: number; processes: Processes }>(`${host(name)}/processes`, { at }),
  processUsage: (name: string, from: number, to: number) =>
    get<{ snapshots: number; firstTime: number; processes: ProcessUsage[] }>(`${host(name)}/process-usage`, { from, to }),
  customMetrics: (name: string) => get<{ names: string[] }>(`${host(name)}/custom-metrics`),
  diskForecasts: (name: string) => get<{ disks: DiskForecast[] }>(`${host(name)}/disk-forecasts`),
  endpoints: (from: number, to: number) => get<{ endpoints: EndpointStatus[] }>('/api/v1/endpoints', { from, to }),
  endpointSeries: (name: string, metric: 'latency' | 'availability', from: number, to: number) =>
    get<SeriesResponse>('/api/v1/endpoints/series', { name, metric, from, to }),
  alerts: (filter: { host?: string; open?: boolean; from?: number; to?: number } = {}) =>
    get<{ alerts: AlertRecord[] }>('/api/v1/alerts', filter),
};
