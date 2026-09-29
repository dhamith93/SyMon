import { api } from './api';

// settings from the client, loaded once logged in
export const appConfig = $state({ refreshSeconds: 15, version: '', collectorVersion: '' });

export function loadConfig() {
  api
    .config()
    .then((config) => Object.assign(appConfig, config))
    .catch(() => {
      // keep the defaults
    });
}
