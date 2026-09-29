import { describe, expect, it } from 'vitest';
import { formatAgo, formatBytes, formatDays, formatDuration, formatNumber, formatPercent, formatRate, formatValue } from './format';

describe('formatBytes', () => {
  it('uses binary units', () => {
    expect(formatBytes(0)).toBe('0 B');
    expect(formatBytes(1023)).toBe('1023 B');
    expect(formatBytes(1536)).toBe('1.5 KiB');
    expect(formatBytes(5 * 1024 ** 3)).toBe('5.0 GiB');
  });

  it('handles bad input', () => {
    expect(formatBytes(NaN)).toBe('–');
  });
});

describe('formatValue', () => {
  it('formats by unit', () => {
    expect(formatRate(2048)).toBe('2.0 KiB/s');
    expect(formatValue(42.4, 'percent')).toBe('42%');
    expect(formatValue(4.25, 'percent')).toBe('4.3%');
    expect(formatValue(512, 'mib')).toBe('512.0 MiB');
    expect(formatValue(48.25, 'celsius')).toBe('48.3 °C');
    expect(formatPercent(12.345, 1)).toBe('12.3%');
  });

  it('compacts large numbers and keeps small ones precise', () => {
    expect(formatNumber(12500)).toBe('12.5K');
    expect(formatNumber(3_400_000)).toBe('3.4M');
    expect(formatNumber(0.52)).toBe('0.52');
    expect(formatNumber(12)).toBe('12');
  });
});

describe('formatDuration', () => {
  it('shows the two largest units', () => {
    expect(formatDuration(45)).toBe('45s');
    expect(formatDuration(4000)).toBe('1h 6m');
    expect(formatDuration(93784)).toBe('1d 2h');
  });

  it('says how long ago', () => {
    expect(formatAgo(0)).toBe('never');
    expect(formatAgo(1000, 1010)).toBe('just now');
    expect(formatAgo(1000, 1000 + 180)).toBe('3m ago');
  });
});

describe('formatDays', () => {
  it('rounds to a unit that fits', () => {
    expect(formatDays(0)).toBe('under a day');
    expect(formatDays(0.4)).toBe('under a day');
    expect(formatDays(1.2)).toBe('about 1 day');
    expect(formatDays(9.4)).toBe('about 9 days');
    expect(formatDays(20.4)).toBe('about 20 days');
    expect(formatDays(35)).toBe('about 5 weeks');
    expect(formatDays(120)).toBe('about 4 months');
  });

  it('handles bad input', () => {
    expect(formatDays(-1)).toBe('–');
    expect(formatDays(NaN)).toBe('–');
  });
});
