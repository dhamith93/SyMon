import { maxSeries, type Aligned, type Value } from './align';
import type { DiskForecast } from './api';

const day = 86400;

export interface Projection {
  data: Aligned;
  from: number;
  to: number;
  // the filling disks, in the order of the chart's series
  disks: DiskForecast[];
}

// Where each filling disk is headed at its growth over the last week. Each
// line runs from now to the day the disk is full, or to the end of the
// chart. The chart spans a bit past the last disk to fill, 7 to 90 days.
// mounts is the series order of the disk space chart, so a disk keeps its
// color there.
export function projectDisks(forecasts: DiskForecast[], now: number, mounts: string[]): Projection | null {
  const rank = (mount: string) => {
    const i = mounts.indexOf(mount);
    return i < 0 ? mounts.length : i;
  };
  const disks = forecasts
    .filter((f) => f.daysToFull !== null)
    .sort((a, b) => rank(a.mount) - rank(b.mount) || a.mount.localeCompare(b.mount))
    .slice(0, maxSeries);
  if (disks.length === 0) return null;

  const longest = Math.max(...disks.map((d) => d.daysToFull ?? 0));
  const horizon = Math.min(90, Math.max(7, Math.ceil(longest * 1.2)));
  const ends = disks.map((d) => now + Math.round(Math.min(d.daysToFull ?? 0, horizon) * day));
  const times = [...new Set([now, ...ends])].sort((a, b) => a - b);

  // undefined between the two ends, so each line joins straight across
  const values: Value[][] = disks.map((d, i) =>
    times.map((time) => {
      if (time === now) return d.usedPct;
      if (time === ends[i]) return Math.min(100, d.usedPct + (d.pctPerDay * (ends[i] - now)) / day);
      return undefined;
    }),
  );
  // a mount the disk chart does not show gets the first free color
  const taken = new Set(disks.map((d) => mounts.indexOf(d.mount)).filter((i) => i >= 0 && i < maxSeries));
  const free = [...Array(maxSeries).keys()].filter((i) => !taken.has(i));
  const colors = disks.map((d) => {
    const i = mounts.indexOf(d.mount);
    return i >= 0 && i < maxSeries ? i : (free.shift() ?? 0);
  });

  return {
    data: { times, labels: disks.map((d) => d.mount), values, hidden: [], colors },
    from: now,
    to: now + horizon * day,
    disks,
  };
}
