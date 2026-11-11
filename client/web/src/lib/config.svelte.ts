import { api } from './api';

// settings from the client, loaded once logged in. clusters are the
// Kubernetes clusters that report and devices the SNMP devices, the nav
// only links to them when there are some.
export const appConfig = $state({ refreshSeconds: 15, version: '', collectorVersion: '', clusters: [] as string[], devices: [] as string[] });

export function loadConfig() {
  api
    .config()
    .then((config) => Object.assign(appConfig, config))
    .catch(() => {
      // keep the defaults
    });
  api
    .clusters()
    .then((list) => (appConfig.clusters = list.clusters.map((c) => c.name)))
    .catch(() => {
      // a collector from before clusters
    });
  api
    .devices()
    .then((list) => (appConfig.devices = list.devices.map((d) => d.name)))
    .catch(() => {
      // a collector from before devices
    });
}
