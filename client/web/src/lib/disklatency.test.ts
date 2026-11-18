import { describe, expect, it } from 'vitest';
import type { DiskLatencyReport } from './api';
import { bucketLabel, bucketSpan, bucketTotals, heatmapRows, quantileSeries } from './disklatency';

// where each of the 19 buckets starts, in ms
const starts = Array.from({ length: 19 }, (_, i) => (i === 0 ? 0 : 0.016 * 2 ** (i - 1)));

function buckets(set: Record<number, number>): number[] {
  return starts.map((_, i) => set[i] ?? 0);
}

const report: DiskLatencyReport = {
  devices: ['/dev/nvme0n1', '/dev/sda'],
  device: '/dev/nvme0n1',
  op: 'read',
  stepSeconds: 60,
  bucketStarts: starts,
  steps: [
    { time: 1000, buckets: buckets({ 2: 90, 10: 10 }), p50: 0.05, p95: 12.3, p99: 15.6 },
    // a step with no reads leaves a gap
    { time: 1060, buckets: buckets({}), p50: 0, p95: 0, p99: 0 },
    { time: 1120, buckets: buckets({ 3: 40 }), p50: 0.096, p95: 0.125, p99: 0.127 },
  ],
};

describe('disk latency', () => {
  it('names buckets by where they start', () => {
    expect(bucketLabel(starts, 0)).toBe('< 16 µs');
    expect(bucketLabel(starts, 6)).toBe('512 µs');
    expect(bucketLabel(starts, 7)).toBe('1.0 ms');
    expect(bucketLabel(starts, 18)).toBe('≥ 2.10 s');
    expect(bucketSpan(starts, 6)).toBe('512 µs to 1.0 ms');
    expect(bucketSpan(starts, 18)).toBe('≥ 2.10 s');
  });

  it('draws the quantiles where there were IOs', () => {
    const lines = quantileSeries(report);
    expect(lines.map((l) => l.label)).toEqual(['p50', 'p95', 'p99']);
    expect(lines[2].points).toEqual([
      [1000, 15.6],
      [1120, 0.127],
    ]);
    expect(quantileSeries({ ...report, steps: [] })).toEqual([]);
  });

  it('shows each bucket as its share of the step, slowest on top', () => {
    const { series, counts } = heatmapRows(report);
    // from the fastest bucket with IOs, 2, to the slowest, 10
    expect(series.map((s) => s.label)).toEqual(['10', '9', '8', '7', '6', '5', '4', '3', '2']);
    expect(series[0].points).toEqual([[1000, 10]]);
    expect(series[7].points).toEqual([[1120, 100]]);
    expect(series[8].points).toEqual([[1000, 90]]);
    // a bucket without IOs stays blank
    expect(series[1].points).toEqual([]);
    expect(counts.get('3')?.get(1120)).toBe(40);
  });

  it('widens a narrow band', () => {
    const one = { ...report, steps: [{ time: 1000, buckets: buckets({ 0: 5 }), p50: 0.008, p95: 0.015, p99: 0.016 }] };
    expect(heatmapRows(one).series.map((s) => s.label)).toEqual(['4', '3', '2', '1', '0']);
    const slow = { ...report, steps: [{ time: 1000, buckets: buckets({ 18: 1 }), p50: 2097, p95: 2097, p99: 2097 }] };
    expect(heatmapRows(slow).series.map((s) => s.label)).toEqual(['18', '17', '16', '15', '14']);
    const middle = { ...report, steps: [{ time: 1000, buckets: buckets({ 9: 1 }), p50: 5, p95: 5, p99: 5 }] };
    expect(heatmapRows(middle).series.map((s) => s.label)).toEqual(['11', '10', '9', '8', '7']);
    expect(heatmapRows({ ...report, steps: [] }).series).toEqual([]);
  });

  it('adds up the range for the table', () => {
    expect(bucketTotals(report)).toEqual([
      { bucket: 10, count: 10, share: 7.142857142857142 },
      { bucket: 3, count: 40, share: 28.57142857142857 },
      { bucket: 2, count: 90, share: 64.28571428571429 },
    ]);
  });
});
