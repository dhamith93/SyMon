import type { ExitGroup, ExitKind } from './api';
import { formatDuration, formatMs } from './format';

// What the eBPF process tracer's exits mean, in words

// Linux numbers, the same on x86, arm and riscv
const signalNames: Record<number, string> = {
  1: 'SIGHUP',
  2: 'SIGINT',
  3: 'SIGQUIT',
  4: 'SIGILL',
  5: 'SIGTRAP',
  6: 'SIGABRT',
  7: 'SIGBUS',
  8: 'SIGFPE',
  9: 'SIGKILL',
  10: 'SIGUSR1',
  11: 'SIGSEGV',
  12: 'SIGUSR2',
  13: 'SIGPIPE',
  14: 'SIGALRM',
  15: 'SIGTERM',
  16: 'SIGSTKFLT',
  17: 'SIGCHLD',
  18: 'SIGCONT',
  19: 'SIGSTOP',
  20: 'SIGTSTP',
  21: 'SIGTTIN',
  22: 'SIGTTOU',
  23: 'SIGURG',
  24: 'SIGXCPU',
  25: 'SIGXFSZ',
  26: 'SIGVTALRM',
  27: 'SIGPROF',
  28: 'SIGWINCH',
  29: 'SIGIO',
  30: 'SIGPWR',
  31: 'SIGSYS',
};

export function signalName(signal: number): string {
  return signalNames[signal] ?? `signal ${signal}`;
}

// signals a program gets when it goes wrong by itself, the same list as
// the collector's crash count
export const crashSignals = new Set([4, 5, 6, 7, 8, 11, 31]);

const sigkill = 9;

export interface ExitMeaning {
  // only the exits worth a look get a badge, the rest are plain text
  badge?: 'critical' | 'warning';
  label: string;
  // 0 is the worst
  rank: number;
}

// describeExit says how a process ended. An OOM kill is a SIGKILL too, and
// a core dump means a crash whatever the signal.
export function describeExit(exit: ExitKind): ExitMeaning {
  if (exit.oom) return { badge: 'critical', label: 'Out of memory, killed', rank: 0 };
  if (exit.coreDumped || crashSignals.has(exit.signal)) {
    const signal = exit.signal ? ` (${signalName(exit.signal)})` : '';
    return { badge: 'critical', label: `Crashed${signal}${exit.coreDumped ? ', core dumped' : ''}`, rank: 1 };
  }
  if (exit.signal === sigkill) return { badge: 'warning', label: 'Killed (SIGKILL)', rank: 2 };
  if (exit.signal) return { label: `Stopped by ${signalName(exit.signal)}`, rank: 3 };
  // what shells answer when they cannot run a command
  if (exit.status === 126) return { label: 'Exit status 126, cannot run', rank: 4 };
  if (exit.status === 127) return { label: 'Exit status 127, command not found', rank: 4 };
  return { label: `Exit status ${exit.status}`, rank: 4 };
}

// the worst first, then the most frequent, then the most recent
export function sortGroups(groups: ExitGroup[]): ExitGroup[] {
  return [...groups].sort((a, b) => describeExit(a).rank - describeExit(b).rank || b.count - a.count || b.last - a.last);
}

export function formatUser(uid: number): string {
  return uid === 0 ? 'root' : `uid ${uid}`;
}

// a count over the seconds the tracer covered, per minute
export function perMinute(count: number, seconds: number): number {
  return seconds > 0 ? (count * 60) / seconds : 0;
}

// how long a process lived. Many that fail do so right after they start,
// so short ones keep their fractions.
export function formatLifetime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return '–';
  if (seconds < 1) return formatMs(seconds * 1000);
  if (seconds < 60) return `${seconds.toFixed(1)} s`;
  return formatDuration(seconds);
}
