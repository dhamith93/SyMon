import type { NamedSeries } from './align';
import type { DiskLatencyReport } from './api';
import { formatMs } from './format';

// A disk's latency histogram from the agent's eBPF tracer, turned into the
// quantile lines and the heatmap rows of the disk latency panel.

const total = (buckets: number[]) => buckets.reduce((sum, count) => sum + count, 0);

// p50, p95 and p99 in ms. A step without IOs leaves a gap.
export function quantileSeries(report: DiskLatencyReport): NamedSeries[] {
  const lines: NamedSeries[] = [
    { label: 'p50', points: [] },
    { label: 'p95', points: [] },
    { label: 'p99', points: [] },
  ];
  for (const step of report.steps) {
    if (total(step.buckets) === 0) continue;
    lines[0].points.push([step.time, step.p50]);
    lines[1].points.push([step.time, step.p95]);
    lines[2].points.push([step.time, step.p99]);
  }
  return lines.filter((line) => line.points.length > 0);
}

// a bucket by where it starts, the first by where it ends: "< 16 µs",
// "512 µs", "1.0 ms", "≥ 2.10 s"
export function bucketLabel(starts: number[], i: number): string {
  if (i === 0) return `< ${formatMs(starts[1] ?? 0)}`;
  if (i === starts.length - 1) return `≥ ${formatMs(starts[i])}`;
  return formatMs(starts[i]);
}

// a bucket's whole span, for tooltips and the table
export function bucketSpan(starts: number[], i: number): string {
  if (i === 0 || i === starts.length - 1) return bucketLabel(starts, i);
  return `${formatMs(starts[i])} to ${formatMs(starts[i + 1])}`;
}

// the heatmap shows at least this many buckets, so one busy bucket still
// reads as a band
const minRows = 5;

export interface LatencyHeatmap {
  // one row per bucket, the slowest on top, labeled by bucket index. A
  // value is the bucket's share of the IOs in that step, in percent. A
  // bucket no IO fell in has no value, so the heatmap leaves it blank.
  series: NamedSeries[];
  // how many IOs each cell is, by bucket index and time
  counts: Map<string, Map<number, number>>;
}

// The heatmap rows, from the fastest to the slowest bucket any IO fell in
export function heatmapRows(report: DiskLatencyReport): LatencyHeatmap {
  const buckets = report.bucketStarts.length;
  let first = buckets;
  let last = -1;
  for (const step of report.steps) {
    step.buckets.forEach((count, i) => {
      if (count > 0) {
        first = Math.min(first, i);
        last = Math.max(last, i);
      }
    });
  }
  if (last < 0) return { series: [], counts: new Map() };
  // widen a narrow band evenly, within the buckets there are
  while (last - first + 1 < Math.min(minRows, buckets)) {
    if (first > 0) first--;
    if (last - first + 1 < minRows && last < buckets - 1) last++;
  }

  const series: NamedSeries[] = [];
  const counts = new Map<string, Map<number, number>>();
  for (let i = last; i >= first; i--) {
    const label = String(i);
    const points: [number, number][] = [];
    const cells = new Map<number, number>();
    for (const step of report.steps) {
      const count = step.buckets[i] ?? 0;
      if (count === 0) continue;
      points.push([step.time, (count / total(step.buckets)) * 100]);
      cells.set(step.time, count);
    }
    series.push({ label, points });
    counts.set(label, cells);
  }
  return { series, counts };
}

// IOs per bucket over the whole range, the slowest first, for the table
export function bucketTotals(report: DiskLatencyReport): { bucket: number; count: number; share: number }[] {
  const sums = report.bucketStarts.map((_, i) => report.steps.reduce((sum, step) => sum + (step.buckets[i] ?? 0), 0));
  const all = total(sums);
  return sums
    .map((count, bucket) => ({ bucket, count, share: all > 0 ? (count / all) * 100 : 0 }))
    .filter((row) => row.count > 0)
    .reverse();
}
