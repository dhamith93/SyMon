import { describe, expect, it } from 'vitest';
import { agentOutdated } from './versions';

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
