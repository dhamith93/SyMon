import { describe, expect, it } from 'vitest';
import type { ExitGroup } from './api';
import { describeExit, formatLifetime, formatUser, perMinute, signalName, sortGroups } from './proctrace';

const kind = { status: 0, signal: 0, coreDumped: false, oom: false };

const group = (name: string, fields: Partial<ExitGroup>): ExitGroup => ({
  ...kind,
  name,
  count: 1,
  first: 1700000000,
  last: 1700000000,
  parent: '',
  lifetimeSeconds: 1,
  ...fields,
});

describe('exits', () => {
  it('names signals', () => {
    expect(signalName(11)).toBe('SIGSEGV');
    expect(signalName(15)).toBe('SIGTERM');
    expect(signalName(64)).toBe('signal 64');
  });

  it('says how a process ended, worst first', () => {
    expect(describeExit({ ...kind, signal: 9, oom: true })).toEqual({ badge: 'critical', label: 'Out of memory, killed', rank: 0 });
    expect(describeExit({ ...kind, signal: 11, coreDumped: true })).toEqual({ badge: 'critical', label: 'Crashed (SIGSEGV), core dumped', rank: 1 });
    expect(describeExit({ ...kind, signal: 6 }).label).toBe('Crashed (SIGABRT)');
    // SIGQUIT is no crash, unless it left a core
    expect(describeExit({ ...kind, signal: 3, coreDumped: true }).badge).toBe('critical');
    expect(describeExit({ ...kind, signal: 9 })).toEqual({ badge: 'warning', label: 'Killed (SIGKILL)', rank: 2 });
    expect(describeExit({ ...kind, signal: 15 })).toEqual({ label: 'Stopped by SIGTERM', rank: 3 });
    expect(describeExit({ ...kind, status: 1 })).toEqual({ label: 'Exit status 1', rank: 4 });
    expect(describeExit({ ...kind, status: 127 }).label).toBe('Exit status 127, command not found');
    expect(describeExit({ ...kind, status: 126 }).label).toBe('Exit status 126, cannot run');
  });

  it('sorts groups by how bad, then how often, then how recent', () => {
    const groups = [
      group('grep', { status: 1, count: 300 }),
      group('nginx', { signal: 15, count: 2 }),
      group('backup', { status: 2, count: 300, last: 1700000100 }),
      group('node', { signal: 11, count: 1 }),
      group('java', { signal: 9, oom: true }),
      group('sleep', { signal: 9, count: 4 }),
    ];
    expect(sortGroups(groups).map((g) => g.name)).toEqual(['java', 'node', 'sleep', 'nginx', 'backup', 'grep']);
  });

  it('formats users and rates', () => {
    expect(formatUser(0)).toBe('root');
    expect(formatUser(998)).toBe('uid 998');
    expect(perMinute(30, 15)).toBe(120);
    expect(perMinute(5, 0)).toBe(0);
    expect(formatLifetime(0.004)).toBe('4.0 ms');
    expect(formatLifetime(2.54)).toBe('2.5 s');
    expect(formatLifetime(93784)).toBe('1d 2h');
  });
});
