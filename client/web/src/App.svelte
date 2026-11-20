<script lang="ts">
  import { auth, checkSession } from './lib/auth.svelte';
  import { appConfig, loadConfig } from './lib/config.svelte';
  import { handleLinkClick, location, match } from './lib/router.svelte';
  import UserMenu from './components/UserMenu.svelte';
  import Account from './pages/Account.svelte';
  import Alerts from './pages/Alerts.svelte';
  import CustomMetrics from './pages/CustomMetrics.svelte';
  import Device from './pages/Device.svelte';
  import Devices from './pages/Devices.svelte';
  import Endpoints from './pages/Endpoints.svelte';
  import Fleet from './pages/Fleet.svelte';
  import Host from './pages/Host.svelte';
  import Kubernetes from './pages/Kubernetes.svelte';
  import Login from './pages/Login.svelte';
  import Notifications from './pages/Notifications.svelte';
  import RuleEditor from './pages/RuleEditor.svelte';
  import Rules from './pages/Rules.svelte';
  import Setup from './pages/Setup.svelte';
  import Tasks from './pages/Tasks.svelte';

  const page = $derived(match(location.path));
  const onAlerts = $derived(page.name === 'alerts');
  const onEndpoints = $derived(page.name === 'endpoints');
  const onKubernetes = $derived(page.name === 'kubernetes');
  const onDevices = $derived(page.name === 'devices' || page.name === 'device');
  const onRules = $derived(page.name === 'rules' || page.name === 'rule' || page.name === 'notifications');
  const onHosts = $derived(!onAlerts && !onEndpoints && !onKubernetes && !onDevices && !onRules && page.name !== 'account');
  // one version when the dashboard and collector match, both when they do not
  const sameVersion = $derived(!appConfig.collectorVersion || appConfig.collectorVersion === appConfig.version);

  checkSession();
  $effect(() => {
    if (auth.state === 'in') loadConfig();
  });
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
      {#if auth.state === 'in'}
        <a href="/" class:active={onHosts} aria-current={onHosts ? 'page' : undefined}>Hosts</a>
        <a href="/endpoints" class:active={onEndpoints} aria-current={onEndpoints ? 'page' : undefined}>Endpoints</a>
        {#if appConfig.clusters.length > 0 || onKubernetes}
          <a href="/kubernetes" class:active={onKubernetes} aria-current={onKubernetes ? 'page' : undefined}>Kubernetes</a>
        {/if}
        {#if appConfig.devices.length > 0 || onDevices}
          <a href="/devices" class:active={onDevices} aria-current={onDevices ? 'page' : undefined}>Devices</a>
        {/if}
        <a href="/alerts" class:active={onAlerts} aria-current={onAlerts ? 'page' : undefined}>Alerts</a>
        <a href="/rules" class:active={onRules} aria-current={onRules ? 'page' : undefined}>Rules</a>
      {/if}
    </nav>
    {#if auth.state !== 'checking'}<UserMenu />{/if}
  </div>
</header>

<main>
  {#if auth.state === 'checking'}
    <!-- nothing to show until the session is known -->
  {:else if auth.state === 'setup'}
    <Setup />
  {:else if auth.state === 'out'}
    <Login />
  {:else if auth.state === 'error'}
    <div class="page">
      <h1>SyMon is not reachable</h1>
      <p class="secondary">{auth.error}</p>
      <button class="control" onclick={checkSession}>Try again</button>
    </div>
  {:else if page.name === 'fleet'}
    <Fleet />
  {:else if page.name === 'host'}
    {#key page.host}<Host host={page.host} />{/key}
  {:else if page.name === 'custom'}
    {#key page.host}<CustomMetrics host={page.host} />{/key}
  {:else if page.name === 'tasks'}
    {#key page.host}<Tasks host={page.host} />{/key}
  {:else if page.name === 'alerts'}
    <Alerts />
  {:else if page.name === 'endpoints'}
    <Endpoints />
  {:else if page.name === 'kubernetes'}
    <Kubernetes />
  {:else if page.name === 'devices'}
    <Devices />
  {:else if page.name === 'device'}
    {#key page.device}<Device device={page.device} />{/key}
  {:else if page.name === 'account'}
    <Account />
  {:else if page.name === 'rules'}
    <Rules />
  {:else if page.name === 'rule'}
    {#key page.id}<RuleEditor id={page.id} />{/key}
  {:else if page.name === 'notifications'}
    <Notifications />
  {:else}
    <div class="page">
      <h1>Page not found</h1>
      <p class="secondary"><a href="/">Back to hosts</a></p>
    </div>
  {/if}
</main>

{#if auth.state === 'in' && appConfig.version}
  <footer class="app-footer muted mono">
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
    min-height: 48px;
    margin: 0 auto;
    padding: 0 16px;
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
    align-self: stretch;
  }

  /* the underline sits on the header's bottom border */
  nav a {
    display: flex;
    align-items: center;
    padding: 14px 10px 12px;
    margin-bottom: -1px;
    border-bottom: 2px solid transparent;
    color: var(--ink-secondary);
    text-decoration: none;
  }

  nav a:hover {
    color: var(--ink);
    border-bottom-color: var(--baseline);
  }

  nav a.active {
    color: var(--ink);
    border-bottom-color: var(--accent);
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
      row-gap: 0;
      padding-top: 8px;
    }

    nav {
      order: 3;
      flex-basis: 100%;
      overflow-x: auto;
      scrollbar-width: none;
    }

    nav a {
      margin-bottom: 0;
      padding: 10px 8px;
    }
  }
</style>
