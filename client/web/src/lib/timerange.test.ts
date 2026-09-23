import { describe, expect, it } from 'vitest';
import { rangeQuery, resolveRange } from './timerange';

const now = 1_700_000_000;

describe('resolveRange', () => {
  it('defaults to a live hour', () => {
    expect(resolveRange(new URLSearchParams(''), now)).toEqual({ preset: '1h', from: now - 3600, to: now, live: true });
  });

  it('reads a preset', () => {
    const range = resolveRange(new URLSearchParams('range=7d'), now);
    expect(range.preset).toBe('7d');
    expect(range.to - range.from).toBe(7 * 86400);
  });

  it('reads a fixed window, which is not live', () => {
    expect(resolveRange(new URLSearchParams('from=100&to=200'), now)).toEqual({ preset: '', from: 100, to: 200, live: false });
  });

  it('ignores a bad window or preset', () => {
    expect(resolveRange(new URLSearchParams('from=300&to=200'), now).preset).toBe('1h');
    expect(resolveRange(new URLSearchParams('range=5y'), now).preset).toBe('1h');
  });
});

describe('rangeQuery', () => {
  it('keeps other parameters and drops the default preset', () => {
    const query = new URLSearchParams('m=queue&from=1&to=2');
    expect(rangeQuery(query, { preset: '1h' })).toBe('?m=queue');
    expect(rangeQuery(query, { preset: '24h' })).toBe('?m=queue&range=24h');
  });

  it('writes a window as whole seconds', () => {
    expect(rangeQuery(new URLSearchParams('range=6h'), { from: 100.7, to: 200.2 })).toBe('?from=100&to=201');
  });
});
