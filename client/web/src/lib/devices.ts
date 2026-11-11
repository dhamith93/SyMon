import { maxSeries } from './align';
import type { DeviceInterface, DeviceState, DeviceSummary } from './api';
import type { Unit } from './format';

// How SNMP devices stand and what their pages chart

export interface Badge {
  status: 'up' | 'warning' | 'critical';
  label: string;
}

// A stale device says nothing about now, so that wins over its last error
export function deviceStatus(device: Pick<DeviceSummary, 'error' | 'stale'>): Badge {
  if (device.stale) return { status: 'warning', label: 'No new reports' };
  if (device.error) return { status: 'critical', label: 'Unreachable' };
  return { status: 'up', label: 'Up' };
}

const kindNames: Record<string, string> = { ups: 'UPS', switch: 'Switch', server: 'Server', nas: 'NAS', other: 'Device' };

// the devices file may name any kind, like printer
export function kindLabel(kind: string): string {
  if (!kind) return 'Device';
  return kindNames[kind] ?? kind[0].toUpperCase() + kind.slice(1);
}

// what a port's state means. Only a link that is up gets a badge, a switch
// has plenty of ports with nothing plugged in.
export function portState(port: DeviceInterface): Badge | string {
  if (port.Up) return { status: 'up', label: 'Up' };
  return port.AdminUp ? 'No link' : 'Switched off';
}

// 1000000000 -> "1 Gbps", 2500000 -> "2.5 Mbps"
export function formatSpeed(bitsPerSecond: number | undefined): string {
  if (!bitsPerSecond) return '–';
  const units = ['bps', 'kbps', 'Mbps', 'Gbps', 'Tbps'];
  let value = bitsPerSecond;
  let unit = 0;
  while (value >= 1000 && unit < units.length - 1) {
    value /= 1000;
    unit++;
  }
  return `${Number(value.toFixed(1))} ${units[unit]}`;
}

// a metric's values by label, from the last good read
export function valuesOf(state: DeviceState, metric: string): Map<string, number> {
  const values = new Map<string, number>();
  for (const v of state.Values ?? []) if (v.Metric === metric) values.set(v.Label ?? '', v.Value);
  return values;
}

// the metrics the profiles produce. Anything else came from the devices
// file's own OIDs.
export const builtinMetrics: Record<string, { label: string; unit: Unit }> = {
  if_rx_bps: { label: 'Traffic in', unit: 'bits' },
  if_tx_bps: { label: 'Traffic out', unit: 'bits' },
  if_rx_errors_ps: { label: 'Errors in, per second', unit: 'number' },
  if_tx_errors_ps: { label: 'Errors out, per second', unit: 'number' },
  if_up: { label: 'Link up', unit: 'number' },
  cpu_pct: { label: 'CPU', unit: 'percent' },
  mem_used_pct: { label: 'Memory used', unit: 'percent' },
  storage_used_pct: { label: 'Storage used', unit: 'percent' },
  ups_on_battery: { label: 'On battery', unit: 'number' },
  ups_charge_pct: { label: 'Battery charge', unit: 'percent' },
  ups_load_pct: { label: 'Load', unit: 'percent' },
  ups_minutes_left: { label: 'Minutes left on battery', unit: 'number' },
  ups_input_volts: { label: 'Input voltage, V', unit: 'number' },
  ups_battery_celsius: { label: 'Battery temperature', unit: 'celsius' },
};

// a unit from the devices file, as a chart unit where there is one
export function chartUnit(unit: string | undefined): Unit {
  switch (unit) {
    case '%':
      return 'percent';
    case '°C':
    case 'C':
      return 'celsius';
    case 'bps':
      return 'bits';
  }
  return 'number';
}

// A chart on a device's page. Each part is one metric, and name replaces
// the empty label of a single value, so two values can share a chart.
export interface ChartSpec {
  key: string;
  title: string;
  unit: Unit;
  yMax?: number;
  note?: string;
  parts: { metric: string; name?: string }[];
  // only these labels, in this order, with these palette slots
  labels?: string[];
  colors?: number[];
}

export interface Section {
  key: 'ups' | 'system' | 'ports' | 'other';
  title: string;
  charts: ChartSpec[];
}

const has = (state: DeviceState, metric: string) => (state.Values ?? []).some((v) => v.Metric === metric);

// customMetrics are the values from the devices file's own OIDs, each with
// its unit
export function customMetrics(state: DeviceState): { metric: string; unit: string }[] {
  const seen = new Map<string, string>();
  for (const v of state.Values ?? []) if (!(v.Metric in builtinMetrics) && !seen.has(v.Metric)) seen.set(v.Metric, v.Unit ?? '');
  return [...seen].map(([metric, unit]) => ({ metric, unit })).sort((a, b) => a.metric.localeCompare(b.metric));
}

// sections lists what a device's page charts, from what its last good read
// has. Port charts show the picked ports, each in its palette slot.
export function sections(state: DeviceState, picked: string[]): Section[] {
  const list: Section[] = [];

  const ups: ChartSpec[] = [];
  if (has(state, 'ups_charge_pct') || has(state, 'ups_load_pct')) {
    ups.push({
      key: 'ups-levels',
      title: 'Battery charge and load',
      unit: 'percent',
      yMax: 100,
      parts: [
        { metric: 'ups_charge_pct', name: 'Charge' },
        { metric: 'ups_load_pct', name: 'Load' },
      ],
    });
  }
  for (const metric of ['ups_minutes_left', 'ups_input_volts', 'ups_battery_celsius']) {
    if (has(state, metric)) ups.push({ key: metric, title: builtinMetrics[metric].label, unit: builtinMetrics[metric].unit, parts: [{ metric }] });
  }
  if (ups.length > 0) list.push({ key: 'ups', title: 'Power', charts: ups });

  const system: ChartSpec[] = [];
  for (const metric of ['cpu_pct', 'mem_used_pct', 'storage_used_pct']) {
    if (has(state, metric)) system.push({ key: metric, title: builtinMetrics[metric].label, unit: 'percent', yMax: 100, parts: [{ metric }] });
  }
  if (system.length > 0) list.push({ key: 'system', title: 'System', charts: system });

  if ((state.Interfaces ?? []).length > 0) {
    const labels = picked.filter((p) => p !== '');
    const colors = labels.map((p) => picked.indexOf(p));
    const port = (metric: string, note?: string): ChartSpec => ({
      key: metric,
      title: builtinMetrics[metric].label,
      unit: builtinMetrics[metric].unit,
      note,
      parts: [{ metric }],
      labels,
      colors,
    });
    list.push({
      key: 'ports',
      title: 'Ports',
      charts: [port('if_rx_bps'), port('if_tx_bps'), port('if_rx_errors_ps', 'Shown when a picked port had errors'), port('if_tx_errors_ps', 'Shown when a picked port had errors')],
    });
  }

  const other = customMetrics(state).map(
    ({ metric, unit }): ChartSpec => ({
      key: `custom-${metric}`,
      title: metric,
      unit: chartUnit(unit),
      note: unit && chartUnit(unit) === 'number' ? unit : undefined,
      parts: [{ metric }],
    }),
  );
  if (other.length > 0) list.push({ key: 'other', title: 'From the devices file', charts: other });
  return list;
}

// Picked ports keep their palette slot while others are ticked and
// unticked: a port's place in the list is its slot, and '' is a free one.
// In the URL that is one ?port= per slot.
export function readPorts(query: URLSearchParams): string[] | null {
  return query.has('port') ? query.getAll('port') : null;
}

export function writePorts(query: URLSearchParams, slots: string[]) {
  query.delete('port');
  const trimmed = [...slots];
  while (trimmed.length > 0 && trimmed[trimmed.length - 1] === '') trimmed.pop();
  // a single empty one says none is picked, not the default
  for (const slot of trimmed.length > 0 ? trimmed : ['']) query.append('port', slot);
}

// toggle unticks a port and frees its slot, or ticks it into the first
// free slot. With every slot taken nothing changes.
export function togglePort(slots: string[], port: string): string[] {
  const next = [...slots];
  const at = next.indexOf(port);
  if (at >= 0) {
    next[at] = '';
    return next;
  }
  const free = next.indexOf('');
  if (free >= 0) next[free] = port;
  else if (next.length < maxSeries) next.push(port);
  return next;
}

// defaultPorts are the busiest ports with a link, at most one per palette
// slot
export function defaultPorts(state: DeviceState): string[] {
  const rx = valuesOf(state, 'if_rx_bps');
  const tx = valuesOf(state, 'if_tx_bps');
  const traffic = (port: DeviceInterface) => (rx.get(port.Name) ?? 0) + (tx.get(port.Name) ?? 0);
  return (state.Interfaces ?? [])
    .filter((p) => p.Up)
    .sort((a, b) => traffic(b) - traffic(a) || a.Name.localeCompare(b.Name))
    .slice(0, maxSeries)
    .map((p) => p.Name);
}
