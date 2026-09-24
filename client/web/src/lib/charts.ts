import { align, named, type Aligned, type NamedSeries } from './align';
import { api } from './api';
import type { Unit } from './format';

// A chart on the host page. A single metric with labels gives one series
// per disk, interface or sensor. Several metrics each give one series,
// named by `name`.
export interface ChartSpec {
  key: string;
  title: string;
  unit: Unit;
  yMax?: number;
  area?: boolean;
  note?: string;
  // left out when the host has no data for it, like temperatures in a VM
  optional?: boolean;
  metrics: { metric: string; name?: string }[];
}

export interface Section {
  title: string;
  charts: ChartSpec[];
}

export const hostSections: Section[] = [
  {
    title: 'CPU',
    charts: [
      { key: 'cpu', title: 'CPU usage', unit: 'percent', yMax: 100, area: true, metrics: [{ metric: 'cpu', name: 'CPU' }] },
      {
        key: 'load',
        title: 'Load average',
        unit: 'number',
        metrics: [
          { metric: 'load1', name: '1 min' },
          { metric: 'load5', name: '5 min' },
          { metric: 'load15', name: '15 min' },
        ],
      },
    ],
  },
  {
    title: 'Memory',
    charts: [
      {
        key: 'memory',
        title: 'Memory and swap used',
        unit: 'percent',
        yMax: 100,
        metrics: [
          { metric: 'memory', name: 'Memory' },
          { metric: 'swap', name: 'Swap' },
        ],
      },
      {
        key: 'pressure',
        title: 'Pressure stall',
        unit: 'percent',
        optional: true,
        note: 'Share of time tasks waited on each resource, 10 second average',
        metrics: [
          { metric: 'psi_cpu', name: 'CPU' },
          { metric: 'psi_memory', name: 'Memory' },
          { metric: 'psi_io', name: 'IO' },
        ],
      },
    ],
  },
  {
    title: 'Disks',
    charts: [
      { key: 'disk_used', title: 'Disk space used', unit: 'percent', yMax: 100, metrics: [{ metric: 'disk_used' }] },
      { key: 'disk_util', title: 'Disk busy time', unit: 'percent', yMax: 100, optional: true, metrics: [{ metric: 'disk_util' }] },
      { key: 'disk_read', title: 'Disk reads', unit: 'rate', optional: true, metrics: [{ metric: 'disk_read' }] },
      { key: 'disk_write', title: 'Disk writes', unit: 'rate', optional: true, metrics: [{ metric: 'disk_write' }] },
    ],
  },
  {
    title: 'Network',
    charts: [
      { key: 'net_rx', title: 'Received', unit: 'rate', metrics: [{ metric: 'net_rx' }] },
      { key: 'net_tx', title: 'Sent', unit: 'rate', metrics: [{ metric: 'net_tx' }] },
      {
        key: 'tcp',
        title: 'TCP connections',
        unit: 'number',
        optional: true,
        metrics: [
          { metric: 'tcp_established', name: 'Established' },
          { metric: 'tcp_time_wait', name: 'Time wait' },
          { metric: 'tcp_close_wait', name: 'Close wait' },
          { metric: 'tcp_listen', name: 'Listening' },
        ],
      },
    ],
  },
  {
    title: 'Containers',
    charts: [
      { key: 'container_cpu', title: 'Container CPU, share of the host', unit: 'percent', optional: true, metrics: [{ metric: 'container_cpu' }] },
      { key: 'container_memory', title: 'Container memory', unit: 'bytes', optional: true, metrics: [{ metric: 'container_memory' }] },
      { key: 'container_rx', title: 'Container traffic received', unit: 'rate', optional: true, note: 'Containers on the host network are left out', metrics: [{ metric: 'container_rx' }] },
      { key: 'container_tx', title: 'Container traffic sent', unit: 'rate', optional: true, note: 'Containers on the host network are left out', metrics: [{ metric: 'container_tx' }] },
    ],
  },
  {
    title: 'Sensors',
    charts: [{ key: 'temperature', title: 'Temperatures', unit: 'celsius', optional: true, metrics: [{ metric: 'temperature' }] }],
  },
];

export async function loadChart(host: string, spec: ChartSpec, from: number, to: number): Promise<Aligned> {
  const responses = await Promise.all(spec.metrics.map((m) => api.series(host, m.metric, from, to)));
  const series: NamedSeries[] = [];
  responses.forEach((response, i) => {
    const name = spec.metrics[i].name;
    series.push(...named(response.series, (label) => name ?? label));
  });
  return align(series);
}

// per core usage for the heatmap, cores in numeric order
export async function loadCores(host: string, from: number, to: number): Promise<NamedSeries[]> {
  const response = await api.series(host, 'cpu_core', from, to, { maxPoints: 300 });
  return named(response.series).sort((a, b) => Number(a.label) - Number(b.label));
}
