<script lang="ts">
  import { appConfig } from './lib/config.svelte';
  import { handleLinkClick, location, match } from './lib/router.svelte';
  import { setTheme, theme, type ThemeChoice } from './lib/theme.svelte';
  import Alerts from './pages/Alerts.svelte';
  import CustomMetrics from './pages/CustomMetrics.svelte';
  import Endpoints from './pages/Endpoints.svelte';
  import Fleet from './pages/Fleet.svelte';
  import Host from './pages/Host.svelte';

  const page = $derived(match(location.path));
  const onAlerts = $derived(page.name === 'alerts');
  const onEndpoints = $derived(page.name === 'endpoints');
  const onHosts = $derived(!onAlerts && !onEndpoints);
  // one version when the dashboard and collector match, both when they do not
  const sameVersion = $derived(!appConfig.collectorVersion || appConfig.collectorVersion === appConfig.version);
</script>

<svelte:document onclick={handleLinkClick} />

<header class="app-header">
  <div class="inner">
    <a class="brand" href="/">
      <svg viewBox="0 0 32 32" width="22" height="22" aria-hidden="true">
        <rect width="32" height="32" rx="7" fill="var(--accent)" />
        <path d="M6 20h5l3-8 4 12 3-7h5" fill="none" stroke="#fff" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" />
      </svg>
      SyMon
    </a>
    <nav>
      <a href="/" class:active={onHosts} aria-current={onHosts ? 'page' : undefined}>Hosts</a>
      <a href="/endpoints" class:active={onEndpoints} aria-current={onEndpoints ? 'page' : undefined}>Endpoints</a>
      <a href="/alerts" class:active={onAlerts} aria-current={onAlerts ? 'page' : undefined}>Alerts</a>
    </nav>
    <label class="theme secondary">
      <span class="sr-only">Theme</span>
      <select class="control" value={theme.choice} onchange={(e) => setTheme(e.currentTarget.value as ThemeChoice)}>
        <option value="system">System theme</option>
        <option value="light">Light</option>
        <option value="dark">Dark</option>
      </select>
    </label>
  </div>
</header>

<main>
  {#if page.name === 'fleet'}
    <Fleet />
  {:else if page.name === 'host'}
    {#key page.host}<Host host={page.host} />{/key}
  {:else if page.name === 'custom'}
    {#key page.host}<CustomMetrics host={page.host} />{/key}
  {:else if page.name === 'alerts'}
    <Alerts />
  {:else if page.name === 'endpoints'}
    <Endpoints />
  {:else}
    <div class="page">
      <h1>Page not found</h1>
      <p class="secondary"><a href="/">Back to hosts</a></p>
    </div>
  {/if}
</main>

{#if appConfig.version}
  <footer class="app-footer muted">
    {#if sameVersion}
      SyMon {appConfig.version}
    {:else}
      Dashboard {appConfig.version} · Collector {appConfig.collectorVersion}
    {/if}
  </footer>
{/if}

<style>
  .app-header {
    background: var(--surface);
    border-bottom: 1px solid var(--border);
  }

  .inner {
    display: flex;
    align-items: center;
    gap: 24px;
    max-width: 1440px;
    margin: 0 auto;
    padding: 10px 16px;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 8px;
    font-weight: 700;
    font-size: 16px;
    color: var(--ink);
    text-decoration: none;
  }

  nav {
    display: flex;
    gap: 4px;
    flex: 1;
  }

  nav a {
    padding: 6px 10px;
    border-radius: 6px;
    color: var(--ink-secondary);
    text-decoration: none;
  }

  nav a:hover {
    background: var(--hover);
  }

  nav a.active {
    color: var(--ink);
    font-weight: 600;
    background: var(--hover);
  }

  main :global(.page) {
    max-width: 1440px;
    margin: 0 auto;
    padding: 20px 16px 48px;
  }

  main :global(.page-header) {
    margin-bottom: 16px;
  }

  main :global(h1) {
    font-size: 22px;
  }

  .app-footer {
    max-width: 1440px;
    margin: 0 auto;
    padding: 0 16px 24px;
    font-size: 12px;
  }

  @media (max-width: 560px) {
    .inner {
      flex-wrap: wrap;
      gap: 10px;
    }

    nav {
      order: 3;
      flex-basis: 100%;
    }
  }
</style>
