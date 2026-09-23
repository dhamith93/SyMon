// The time range every chart on a page shares, kept in the URL as
// ?range=1h for a live preset or ?from=..&to=.. for a fixed window

export interface Preset {
  key: string;
  label: string;
  seconds: number;
}

export const presets: Preset[] = [
  { key: '15m', label: '15 min', seconds: 15 * 60 },
  { key: '1h', label: '1 hour', seconds: 3600 },
  { key: '6h', label: '6 hours', seconds: 6 * 3600 },
  { key: '24h', label: '24 hours', seconds: 86400 },
  { key: '7d', label: '7 days', seconds: 7 * 86400 },
  { key: '30d', label: '30 days', seconds: 30 * 86400 },
];

export const defaultPreset = '1h';

export interface TimeRange {
  // a preset key, or '' for a fixed window
  preset: string;
  from: number;
  to: number;
  // live ranges move with the clock and refresh
  live: boolean;
}

export function resolveRange(query: URLSearchParams, now = Math.floor(Date.now() / 1000)): TimeRange {
  const from = Number(query.get('from'));
  const to = Number(query.get('to'));
  if (Number.isInteger(from) && Number.isInteger(to) && from > 0 && to > from) {
    return { preset: '', from, to, live: false };
  }
  const preset = presets.find((p) => p.key === query.get('range')) ?? presets.find((p) => p.key === defaultPreset)!;
  return { preset: preset.key, from: now - preset.seconds, to: now, live: true };
}

// the query string for a range, keeping any other parameters
export function rangeQuery(query: URLSearchParams, range: { preset: string } | { from: number; to: number }): string {
  const next = new URLSearchParams(query);
  next.delete('range');
  next.delete('from');
  next.delete('to');
  if ('preset' in range) {
    if (range.preset !== defaultPreset) next.set('range', range.preset);
  } else {
    next.set('from', String(Math.floor(range.from)));
    next.set('to', String(Math.ceil(range.to)));
  }
  const text = next.toString();
  return text ? `?${text}` : '';
}

// value for an <input type="datetime-local">, in the browser's time zone
export function toLocalInput(unixSeconds: number): string {
  const date = new Date(unixSeconds * 1000);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

export function fromLocalInput(value: string): number {
  const time = new Date(value).getTime();
  return Number.isNaN(time) ? 0 : Math.floor(time / 1000);
}
