import { describe, expect, it } from 'vitest';
import type { DeviceState } from './api';
import { chartUnit, customMetrics, defaultPorts, deviceStatus, formatSpeed, kindLabel, portState, readPorts, sections, togglePort, valuesOf, writePorts } from './devices';

const port = (Name: string, Up: boolean, AdminUp = true) => ({ Name, Up, AdminUp, SpeedBps: 1e9 });

const switchState: DeviceState = {
  Device: 'sw1',
  Time: 1700000000,
  IntervalSeconds: 60,
  Kind: 'switch',
  Address: '10.0.0.2:161',
  System: { Descr: 'Cisco IOS', UptimeSeconds: 86400 },
  Interfaces: [port('Gi1', true), port('Gi2', true), port('Gi3', false), port('Gi4', false, false), port('Gi5', true)],
  Values: [
    { Metric: 'if_rx_bps', Label: 'Gi1', Unit: 'bps', Value: 1000 },
    { Metric: 'if_tx_bps', Label: 'Gi1', Unit: 'bps', Value: 1000 },
    { Metric: 'if_rx_bps', Label: 'Gi2', Unit: 'bps', Value: 5e6 },
    { Metric: 'if_rx_bps', Label: 'Gi5', Unit: 'bps', Value: 0 },
    { Metric: 'if_up', Label: 'Gi1', Value: 1 },
  ],
};

const upsState: DeviceState = {
  Device: 'ups1',
  Time: 1700000000,
  IntervalSeconds: 60,
  Kind: 'ups',
  Address: '10.0.0.3:161',
  System: {},
  Values: [
    { Metric: 'ups_on_battery', Value: 0 },
    { Metric: 'ups_charge_pct', Unit: '%', Value: 100 },
    { Metric: 'ups_minutes_left', Unit: 'min', Value: 42 },
    { Metric: 'nas_temp', Unit: '°C', Value: 41 },
    { Metric: 'fan_rpm', Unit: 'rpm', Value: 1200 },
  ],
};

describe('device status', () => {
  it('puts a stale device before its last error', () => {
    expect(deviceStatus({ error: '', stale: false })).toEqual({ status: 'up', label: 'Up' });
    expect(deviceStatus({ error: 'cannot reach 10.0.0.3', stale: false })).toEqual({ status: 'critical', label: 'Unreachable' });
    expect(deviceStatus({ error: 'cannot reach 10.0.0.3', stale: true }).label).toBe('No new reports');
  });

  it('names kinds and ports', () => {
    expect(kindLabel('ups')).toBe('UPS');
    expect(kindLabel('nas')).toBe('NAS');
    expect(kindLabel('printer')).toBe('Printer');
    expect(kindLabel('')).toBe('Device');
    expect(portState(port('Gi1', true))).toEqual({ status: 'up', label: 'Up' });
    expect(portState(port('Gi3', false))).toBe('No link');
    expect(portState(port('Gi4', false, false))).toBe('Switched off');
    expect(formatSpeed(1e9)).toBe('1 Gbps');
    expect(formatSpeed(2.5e6)).toBe('2.5 Mbps');
    expect(formatSpeed(undefined)).toBe('–');
  });
});

describe('device values', () => {
  it('reads a metric by label', () => {
    const rx = valuesOf(switchState, 'if_rx_bps');
    expect(rx.get('Gi2')).toBe(5e6);
    expect(rx.has('Gi3')).toBe(false);
    expect(valuesOf(upsState, 'ups_charge_pct').get('')).toBe(100);
  });

  it('tells the devices file metrics apart and charts known units', () => {
    expect(customMetrics(upsState)).toEqual([
      { metric: 'fan_rpm', unit: 'rpm' },
      { metric: 'nas_temp', unit: '°C' },
    ]);
    expect(chartUnit('%')).toBe('percent');
    expect(chartUnit('°C')).toBe('celsius');
    expect(chartUnit('rpm')).toBe('number');
  });
});

describe('device sections', () => {
  it('charts a UPS by what it reports', () => {
    const list = sections(upsState, []);
    expect(list.map((s) => s.key)).toEqual(['ups', 'other']);
    expect(list[0].charts.map((c) => c.key)).toEqual(['ups-levels', 'ups_minutes_left']);
    expect(list[0].charts[0].parts.map((p) => p.name)).toEqual(['Charge', 'Load']);
    const other = list[1].charts;
    expect(other.map((c) => [c.title, c.unit, c.note])).toEqual([
      ['fan_rpm', 'number', 'rpm'],
      ['nas_temp', 'celsius', undefined],
    ]);
  });

  it('charts the picked ports in their slots', () => {
    const [ports] = sections(switchState, ['Gi2', '', 'Gi1']);
    expect(ports.key).toBe('ports');
    expect(ports.charts[0].labels).toEqual(['Gi2', 'Gi1']);
    expect(ports.charts[0].colors).toEqual([0, 2]);
    expect(ports.charts[0].unit).toBe('bits');
  });
});

describe('picking ports', () => {
  it('starts with the busiest ports that have a link', () => {
    expect(defaultPorts(switchState)).toEqual(['Gi2', 'Gi1', 'Gi5']);
  });

  it('keeps every picked port in its slot', () => {
    let slots = ['Gi2', 'Gi1', 'Gi5'];
    slots = togglePort(slots, 'Gi1');
    expect(slots).toEqual(['Gi2', '', 'Gi5']);
    slots = togglePort(slots, 'Gi3');
    expect(slots).toEqual(['Gi2', 'Gi3', 'Gi5']);
    slots = togglePort(slots, 'Gi4');
    expect(slots).toEqual(['Gi2', 'Gi3', 'Gi5', 'Gi4']);
    const full = ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h'];
    expect(togglePort(full, 'i')).toEqual(full);
  });

  it('keeps the slots in the URL', () => {
    const query = new URLSearchParams('range=6h');
    expect(readPorts(query)).toBeNull();
    writePorts(query, ['Gi2', '', 'Gi1/0/24', '']);
    expect(query.toString()).toBe('range=6h&port=Gi2&port=&port=Gi1%2F0%2F24');
    expect(readPorts(query)).toEqual(['Gi2', '', 'Gi1/0/24']);
    writePorts(query, ['', '']);
    expect(readPorts(query)).toEqual(['']);
  });
});
