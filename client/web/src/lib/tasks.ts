import type { TaskRun } from './api';
import { formatDuration } from './format';

// How a run stands, in words and as a tone for its badge. busy is a run
// that is still waiting or going, which is no status color.
export type RunTone = 'good' | 'bad' | 'warning' | 'busy';

export function describeRun(run: TaskRun): { tone: RunTone; label: string } {
  switch (run.status) {
    case 'queued':
      return { tone: 'busy', label: 'Waiting for the agent' };
    case 'running':
      return { tone: 'busy', label: 'Running' };
    case 'succeeded':
      return { tone: 'good', label: 'Succeeded' };
    case 'timed_out':
      return { tone: 'bad', label: 'Timed out' };
    case 'expired':
      return { tone: 'warning', label: 'Not picked up' };
    case 'lost':
      return { tone: 'warning', label: 'Lost' };
    case 'failed':
      // a run that never started has no exit code worth showing
      return { tone: 'bad', label: run.error || run.exitCode < 0 ? 'Failed' : `Failed, exit code ${run.exitCode}` };
    default:
      return { tone: 'warning', label: run.status };
  }
}

// a host runs one task at a time, so one of these blocks the next run
export function isActive(run: TaskRun): boolean {
  return run.status === 'queued' || run.status === 'running';
}

// how long a run took, or has taken so far
export function runDuration(run: TaskRun, now: number): string {
  if (!run.startedAt) return '–';
  const seconds = (run.finishedAt || now) - run.startedAt;
  return seconds < 1 ? 'under 1s' : formatDuration(seconds);
}

// each task's newest run. runs come newest first.
export function lastRuns(runs: TaskRun[]): Map<string, TaskRun> {
  const last = new Map<string, TaskRun>();
  for (const run of runs) {
    if (!last.has(run.task)) last.set(run.task, run);
  }
  return last;
}
