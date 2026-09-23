import type { SeriesData } from './api';

export interface NamedSeries {
  label: string;
  points: [number, number][];
}

// uPlot joins a line over undefined values and breaks it at null
export type Value = number | null | undefined;

export interface Aligned {
  times: number[];
  labels: string[];
  // one array per series. undefined where a series has no point at a time
  // another series has, null where no series had data (an outage).
  values: Value[][];
  // labels left out because of the series cap
  hidden: string[];
}

// the categorical palette has 8 slots, more series than that do not get
// a color of their own
export const maxSeries = 8;

// a gap between points longer than this many typical gaps is an outage
const gapFactor = 3;

// Puts several series on one shared, sorted time axis. Series keep the
// order they are given in, so each keeps its color between refreshes.
export function align(series: NamedSeries[]): Aligned {
  const shown = series.slice(0, maxSeries);
  const hidden = series.slice(maxSeries).map((s) => s.label);

  const timeSet = new Set<number>();
  for (const s of shown) {
    for (const [time] of s.points) timeSet.add(time);
  }
  const times = withGaps([...timeSet].sort((a, b) => a - b));
  const index = new Map(times.map((time, i) => [time, i]));

  const values = shown.map((s) => {
    const row: Value[] = times.map((time) => (Number.isInteger(time) ? undefined : null));
    for (const [time, value] of s.points) row[index.get(time)!] = value;
    return row;
  });
  return { times, labels: shown.map((s) => s.label), values, hidden };
}

// Adds a time halfway into each outage. Real times are whole seconds, so
// the added ones are told apart by their half second.
function withGaps(times: number[]): number[] {
  if (times.length < 3) return times;
  const deltas = times.slice(1).map((time, i) => time - times[i]);
  const typical = [...deltas].sort((a, b) => a - b)[Math.floor(deltas.length / 2)];
  const out = [times[0]];
  for (let i = 1; i < times.length; i++) {
    if (deltas[i - 1] > typical * gapFactor) out.push(Math.floor((times[i - 1] + times[i]) / 2) + 0.5);
    out.push(times[i]);
  }
  return out;
}

// Turns API series into named series. rename maps a label to its display
// name, for example a mount point to "/ (root)".
export function named(series: SeriesData[], rename: (label: string) => string = (label) => label): NamedSeries[] {
  return series.map((s) => ({ label: rename(s.label), points: s.points }));
}
