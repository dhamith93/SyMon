import { describe, expect, it } from 'vitest';
import type { HostSummary } from './api';
import { agentOutdated, agentState } from './versions';

describe('agentOutdated', () => {
  it('compares with the build the dashboard hands out', () => {
    expect(agentOutdated('v3.1.0', 'v3.1.0')).toBe(false);
    expect(agentOutdated('v3.0.0-17-g543bc75', 'v3.1.0')).toBe(true);
    expect(agentOutdated('', 'v3.1.0')).toBe(true);
    expect(agentOutdated(undefined, 'v3.1.0')).toBe(true);
  });

  it('says nothing before the dashboard version is known', () => {
    expect(agentOutdated('', '')).toBe(false);
  });
});

describe('agentState', () => {
  const host = (fields: Partial<HostSummary>) =>
    ({ agentVersion: 'v3.1.0', canUpdate: true, updateVersion: '', updateError: '', ...fields }) as HostSummary;

  it('follows the update from asked to done', () => {
    expect(agentState(host({ agentVersion: 'v3.2.0' }), 'v3.2.0')).toBe('current');
    expect(agentState(host({}), 'v3.2.0')).toBe('available');
    expect(agentState(host({ updateVersion: 'v3.2.0' }), 'v3.2.0')).toBe('requested');
    expect(agentState(host({ updateVersion: 'v3.2.0', updateError: 'not signed' }), 'v3.2.0')).toBe('failed');
  });

  it('sends agents from before updates to the install command', () => {
    expect(agentState(host({ agentVersion: '', canUpdate: false }), 'v3.2.0')).toBe('manual');
  });
});
