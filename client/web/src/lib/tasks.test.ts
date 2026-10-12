import { describe, expect, it } from 'vitest';
import type { TaskRun } from './api';
import { describeRun, isActive, lastRuns, runDuration } from './tasks';

function run(fields: Partial<TaskRun>): TaskRun {
  return {
    id: 1,
    host: 'web1',
    task: 'Restart nginx',
    status: 'succeeded',
    requestedBy: 'alice',
    requestedAt: 1000,
    startedAt: 1001,
    finishedAt: 1004,
    exitCode: 0,
    error: '',
    truncated: false,
    ...fields,
  };
}

describe('describeRun', () => {
  it('says how a run stands', () => {
    expect(describeRun(run({ status: 'queued' }))).toEqual({ tone: 'busy', label: 'Waiting for the agent' });
    expect(describeRun(run({ status: 'running' }))).toEqual({ tone: 'busy', label: 'Running' });
    expect(describeRun(run({}))).toEqual({ tone: 'good', label: 'Succeeded' });
    expect(describeRun(run({ status: 'timed_out', exitCode: -1 }))).toEqual({ tone: 'bad', label: 'Timed out' });
    expect(describeRun(run({ status: 'expired' })).tone).toBe('warning');
    expect(describeRun(run({ status: 'lost' })).tone).toBe('warning');
  });

  it('shows the exit code of a command that failed', () => {
    expect(describeRun(run({ status: 'failed', exitCode: 3 })).label).toBe('Failed, exit code 3');
  });

  it('leaves the exit code out when the task never ran or was killed', () => {
    expect(describeRun(run({ status: 'failed', exitCode: -1, error: 'the task changed on the host' })).label).toBe('Failed');
    expect(describeRun(run({ status: 'failed', exitCode: -1 })).label).toBe('Failed');
  });

  it('does not hide a status it does not know', () => {
    expect(describeRun(run({ status: 'paused' }))).toEqual({ tone: 'warning', label: 'paused' });
  });
});

describe('isActive', () => {
  it('is true while a run waits or goes', () => {
    expect(isActive(run({ status: 'queued' }))).toBe(true);
    expect(isActive(run({ status: 'running' }))).toBe(true);
    expect(isActive(run({ status: 'failed' }))).toBe(false);
  });
});

describe('runDuration', () => {
  it('measures from start to finish', () => {
    expect(runDuration(run({}), 2000)).toBe('3s');
    expect(runDuration(run({ startedAt: 1000, finishedAt: 1000 }), 2000)).toBe('under 1s');
  });

  it('counts up while a run is going', () => {
    expect(runDuration(run({ status: 'running', finishedAt: 0 }), 1091)).toBe('1m');
  });

  it('has nothing for a run that never started', () => {
    expect(runDuration(run({ status: 'expired', startedAt: 0, finishedAt: 1200 }), 2000)).toBe('–');
  });
});

describe('lastRuns', () => {
  it('keeps the newest run of each task', () => {
    const runs = [run({ id: 3, task: 'a' }), run({ id: 2, task: 'b' }), run({ id: 1, task: 'a' })];
    const last = lastRuns(runs);
    expect(last.get('a')?.id).toBe(3);
    expect(last.get('b')?.id).toBe(2);
    expect(last.size).toBe(2);
  });
});
