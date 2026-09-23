// Formatting for values shown in charts, tables and tiles

const binaryUnits = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB'];

export function formatBytes(bytes: number, digits = 1): string {
  if (!Number.isFinite(bytes)) return '–';
  let value = Math.abs(bytes);
  let unit = 0;
  while (value >= 1024 && unit < binaryUnits.length - 1) {
    value /= 1024;
    unit++;
  }
  const sign = bytes < 0 ? '-' : '';
  return `${sign}${unit === 0 ? value.toFixed(0) : value.toFixed(digits)} ${binaryUnits[unit]}`;
}

export function formatRate(bytesPerSecond: number): string {
  return `${formatBytes(bytesPerSecond)}/s`;
}

// memory and swap arrive in MiB
export function formatMiB(mib: number): string {
  return formatBytes(mib * 1024 * 1024);
}

export function formatPercent(value: number, digits = 0): string {
  if (!Number.isFinite(value)) return '–';
  return `${value.toFixed(digits)}%`;
}

export function formatNumber(value: number, digits = 1): string {
  if (!Number.isFinite(value)) return '–';
  const abs = Math.abs(value);
  if (abs >= 1e9) return `${(value / 1e9).toFixed(digits)}B`;
  if (abs >= 1e6) return `${(value / 1e6).toFixed(digits)}M`;
  if (abs >= 1e4) return `${(value / 1e3).toFixed(digits)}K`;
  if (Number.isInteger(value)) return value.toLocaleString();
  return value.toFixed(abs < 10 ? 2 : digits);
}

export function formatCelsius(value: number): string {
  if (!Number.isFinite(value)) return '–';
  return `${value.toFixed(1)} °C`;
}

// 93784 -> "1d 2h", 4000 -> "1h 6m", 45 -> "45s"
export function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return '–';
  const s = Math.floor(seconds);
  const days = Math.floor(s / 86400);
  const hours = Math.floor((s % 86400) / 3600);
  const minutes = Math.floor((s % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${minutes}m`;
  if (minutes > 0) return `${minutes}m`;
  return `${s}s`;
}

// "just now", "3m ago", "2h ago", "4d ago"
export function formatAgo(unixSeconds: number, now = Date.now() / 1000): string {
  if (!unixSeconds) return 'never';
  const diff = now - unixSeconds;
  if (diff < 30) return 'just now';
  return `${formatDuration(diff)} ago`;
}

export function formatDateTime(unixSeconds: number): string {
  if (!unixSeconds) return '–';
  return new Date(unixSeconds * 1000).toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

// time on a chart axis or tooltip, with the date only when the range spans days
export function formatTime(unixSeconds: number, spanSeconds: number): string {
  const date = new Date(unixSeconds * 1000);
  if (spanSeconds > 2 * 86400) {
    return date.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
  }
  return date.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: spanSeconds < 3600 ? '2-digit' : undefined });
}

export type Unit = 'percent' | 'rate' | 'bytes' | 'mib' | 'celsius' | 'number';

export function formatValue(value: number, unit: Unit): string {
  switch (unit) {
    case 'percent':
      return formatPercent(value, value < 10 ? 1 : 0);
    case 'rate':
      return formatRate(value);
    case 'bytes':
      return formatBytes(value);
    case 'mib':
      return formatMiB(value);
    case 'celsius':
      return formatCelsius(value);
    default:
      return formatNumber(value);
  }
}

// Labels for axis ticks. All ticks share the decimals their spacing needs,
// so an axis reads 0.5, 1.0, 1.5 and never 0.50, 1, 1.50.
export function formatAxis(ticks: number[], unit: Unit): string[] {
  const step = ticks.length > 1 ? Math.abs(ticks[1] - ticks[0]) : Math.abs(ticks[0] ?? 1);
  const digits = step >= 1 || step === 0 ? 0 : step >= 0.1 ? 1 : 2;
  return ticks.map((value) => {
    switch (unit) {
      case 'percent':
        return `${value.toFixed(digits)}%`;
      case 'celsius':
        return `${value.toFixed(digits)} °C`;
      case 'number':
        return Math.abs(value) >= 1e4 ? formatNumber(value) : value.toFixed(digits);
      default:
        return formatValue(value, unit);
    }
  });
}
