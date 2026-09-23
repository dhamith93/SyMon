import { api } from './api';

// settings from the client, loaded once
export const appConfig = $state({ refreshSeconds: 15 });

api
  .config()
  .then((config) => (appConfig.refreshSeconds = config.refreshSeconds))
  .catch(() => {
    // keep the default
  });
