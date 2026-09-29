import { describe, expect, it } from 'vitest';
import type { DiskForecast } from './api';
import { projectDisks } from './projection';

const now = 1_700_000_000;
const day = 86400;

function disk(mount: string, usedPct: number, pctPerDay: number, daysToFull: number | null): DiskForecast {
  return { device: `/dev/${mount}`, mount, usedPct, pctPerDay, bytesPerDay: 1e9, daysToFull };
}

describe('projectDisks', () => {
  it('leaves out disks that are not filling up', () => {
    expect(projectDisks([disk('/', 40, 0, null)], now, ['/'])).toBeNull();
  });

  it('runs each line from now to the day the disk is full', () => {
    const projection = projectDisks([disk('/data', 60, 2, 20), disk('/', 40, 0, null)], now, ['/', '/data'])!;
    expect(projection.from).toBe(now);
    // a bit past the 20 days, so the end of the line shows
    expect(projection.to).toBe(now + 24 * day);
    expect(projection.data.labels).toEqual(['/data']);
    expect(projection.data.times).toEqual([now, now + 20 * day]);
    expect(projection.data.values).toEqual([[60, 100]]);
    // /data keeps its color from the disk space chart, where it is second
    expect(projection.data.colors).toEqual([1]);
  });

  it('stops lines at the end of the chart', () => {
    const projection = projectDisks([disk('/', 50, 0.5, 100), disk('/var', 80, 4, 5)], now, ['/', '/var'])!;
    expect(projection.to).toBe(now + 90 * day);
    expect(projection.data.times).toEqual([now, now + 5 * day, now + 90 * day]);
    expect(projection.data.values).toEqual([
      [50, undefined, 95],
      [80, 100, undefined],
    ]);
  });

  it('shows at least a week', () => {
    const projection = projectDisks([disk('/', 99, 2, 0.5)], now, ['/'])!;
    expect(projection.to).toBe(now + 7 * day);
  });

  it('gives a disk the disk chart does not show a free color', () => {
    const projection = projectDisks([disk('/new', 10, 1, 90), disk('/', 50, 1, 50)], now, ['/'])!;
    expect(projection.data.labels).toEqual(['/', '/new']);
    expect(projection.data.colors).toEqual([0, 1]);
  });
});
