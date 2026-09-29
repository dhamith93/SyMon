import { api } from './api';

// settings from the client, loaded once
export const appConfig = $state({ refreshSeconds: 15, version: '', collectorVersion: '' });

api
  .config()
  .then((config) => Object.assign(appConfig, config))
  .catch(() => {
    // keep the default
  });
