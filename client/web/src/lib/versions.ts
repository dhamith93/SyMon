import type { HostSummary } from './api';

// An agent is outdated when it is not the build the dashboard hands out,
// which is what the install script upgrades it to. Agents from before
// versions send none.
export function agentOutdated(agentVersion: string | undefined, dashboardVersion: string): boolean {
  return dashboardVersion !== '' && agentVersion !== dashboardVersion;
}

// Where a host's agent stands:
//   current   runs the build the dashboard hands out
//   available can be updated from the dashboard
//   requested an update was asked for and the agent has not picked it up
//   failed    the agent tried the update and it did not work
//   manual    too old to update itself, needs the install command once
export type AgentState = 'current' | 'available' | 'requested' | 'failed' | 'manual';

export function agentState(host: HostSummary, dashboardVersion: string): AgentState {
  if (!agentOutdated(host.agentVersion, dashboardVersion)) return 'current';
  if (!host.canUpdate) return 'manual';
  if (host.updateVersion && host.updateError) return 'failed';
  if (host.updateVersion) return 'requested';
  return 'available';
}
