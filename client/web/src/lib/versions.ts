// An agent is outdated when it is not the build the dashboard hands out,
// which is what the install script upgrades it to. Agents from before
// versions send none.
export function agentOutdated(agentVersion: string | undefined, dashboardVersion: string): boolean {
  return dashboardVersion !== '' && agentVersion !== dashboardVersion;
}
