<script lang="ts">
  import { untrack } from 'svelte';
  import { api, type HostSummary } from '../lib/api';
  import { appConfig } from '../lib/config.svelte';
  import { formatAgo, formatDuration, formatRate } from '../lib/format';
  import { poll } from '../lib/poll';
  import { hostPath } from '../lib/router.svelte';
  import Meter from '../components/Meter.svelte';
  import Sparkline from '../components/Sparkline.svelte';
  import StatTile from '../components/StatTile.svelte';
  import StatusBadge from '../components/StatusBadge.svelte';

  let hosts = $state<HostSummary[]>([]);
  let loaded = $state(false);
  let error = $state('');
  let search = $state('');
  let sortBy = $state<'name' | 'status' | 'cpu' | 'memory' | 'disk' | 'alerts'>('status');
  let sparklines = $state<Record<string, [number, number][]>>({});

  async function loadFleet() {
    try {
      hosts = (await api.fleet()).hosts;
      error = '';
    } catch (e) {
      error = (e as Error).message;
    } finally {
      loaded = true;
    }
  }

  // one small request per host, so these refresh once a minute
  async function loadSparklines() {
    const now = Math.floor(Date.now() / 1000);
    const next: Record<string, [number, number][]> = {};
    await Promise.all(
      hosts.map(async (host) => {
        try {
          const response = await api.series(host.name, 'cpu', now - 3600, now, { maxPoints: 60 });
          next[host.name] = response.series[0]?.points ?? [];
        } catch {
          next[host.name] = [];
        }
      }),
    );
    sparklines = next;
  }

  $effect(() => poll(loadFleet, appConfig.refreshSeconds));
  $effect(() => {
    if (!loaded) return;
    // untracked, so a fleet refresh does not restart this timer
    return poll(() => untrack(loadSparklines), 60);
  });

  const up = $derived(hosts.filter((h) => h.up).length);
  const openAlerts = $derived(hosts.reduce((sum, h) => sum + h.activeAlerts, 0));

  const visible = $derived.by(() => {
    const term = search.trim().toLowerCase();
    const list = hosts.filter((h) => !term || h.name.toLowerCase().includes(term) || h.os.toLowerCase().includes(term));
    const by: Record<typeof sortBy, (a: HostSummary, b: HostSummary) => number> = {
      name: (a, b) => a.name.localeCompare(b.name),
      // down hosts and hosts with alerts first
      status: (a, b) => Number(a.up) - Number(b.up) || b.activeAlerts - a.activeAlerts || a.name.localeCompare(b.name),
      cpu: (a, b) => b.cpuPct - a.cpuPct,
      memory: (a, b) => b.memUsedPct - a.memUsedPct,
      disk: (a, b) => b.diskUsedPct - a.diskUsedPct,
      alerts: (a, b) => b.activeAlerts - a.activeAlerts,
    };
    return list.sort(by[sortBy]);
  });
</script>

<div class="page">
  <header class="page-header">
    <h1>Hosts</h1>
  </header>

  <section class="tiles" aria-label="Fleet summary">
    <StatTile label="Hosts" value={hosts.length} />
    <StatTile label="Up" value={up} />
    <StatTile label="Down" value={hosts.length - up}>
      {#snippet extra()}
        {#if hosts.length - up > 0}<StatusBadge status="down" label="Not reporting" />{/if}
      {/snippet}
    </StatTile>
    <StatTile label="Open alerts" value={openAlerts}>
      {#snippet extra()}
        {#if openAlerts > 0}<a href="/alerts">See alerts</a>{/if}
      {/snippet}
    </StatTile>
  </section>

  <div class="filters">
    <input class="control search" type="search" placeholder="Search hosts" aria-label="Search hosts" bind:value={search} />
    <label class="secondary">
      Sort by
      <select class="control" bind:value={sortBy}>
        <option value="status">Status</option>
        <option value="name">Name</option>
        <option value="cpu">CPU</option>
        <option value="memory">Memory</option>
        <option value="disk">Disk</option>
        <option value="alerts">Alerts</option>
      </select>
    </label>
  </div>

  {#if error}
    <p class="card message">Could not load hosts: {error}</p>
  {:else if loaded && hosts.length === 0}
    <div class="card message">
      <h2>No agents yet</h2>
      <p class="secondary">
        Install the agent on a server, set <code>SYMON_KEY</code> and <code>SYMON_COLLECTOR_ENDPOINT</code>, then run
        <code>agent -init</code> once and start it.
      </p>
    </div>
  {:else if loaded && visible.length === 0}
    <p class="card message secondary">No hosts match "{search}".</p>
  {/if}

  <div class="grid">
    {#each visible as host (host.name)}
      <a class="host card" href={hostPath(host.name)}>
        <div class="top">
          <span class="name">{host.name}</span>
          <StatusBadge status={host.up ? 'up' : 'down'} />
        </div>
        <div class="meta muted">
          {host.os || 'Unknown OS'}
          {#if host.up}· up {formatDuration(host.uptimeSeconds)}{:else}· last seen {formatAgo(host.lastSeen)}{/if}
        </div>

        {#if host.time}
          <div class="meters">
            <Meter label="CPU" value={host.cpuPct} />
            <Meter label="Memory" value={host.memUsedPct} />
            <Meter label="Disk" value={host.diskUsedPct} />
          </div>
          <div class="bottom">
            <span class="net secondary num">↓ {formatRate(host.rxBps)} · ↑ {formatRate(host.txBps)}</span>
            <Sparkline points={sparklines[host.name] ?? []} label="CPU over the last hour" />
          </div>
        {:else}
          <p class="muted nodata">No data received yet.</p>
        {/if}

        {#if host.activeAlerts > 0}
          <div class="alerts">
            <StatusBadge
              status={host.worstSeverity === 2 ? 'critical' : 'warning'}
              label="{host.activeAlerts} open alert{host.activeAlerts === 1 ? '' : 's'}"
            />
          </div>
        {/if}
      </a>
    {/each}
  </div>
</div>

<style>
  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: 12px;
    margin-bottom: 16px;
  }

  .filters {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px;
    margin-bottom: 16px;
  }

  .filters label {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .search {
    width: min(320px, 100%);
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
    gap: 12px;
  }

  .host {
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 14px;
    color: inherit;
    text-decoration: none;
    transition: border-color 0.15s;
  }

  .host:hover {
    border-color: var(--baseline);
    text-decoration: none;
  }

  .top {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
  }

  .name {
    font-weight: 600;
    font-size: 15px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .meta {
    font-size: 12px;
    margin-top: -6px;
  }

  .meters {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .bottom {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
    font-size: 12px;
  }

  .nodata {
    margin: 0;
    font-size: 12px;
  }

  .message {
    padding: 20px;
    margin: 0 0 16px;
  }

  .message p {
    margin: 6px 0 0;
  }

  code {
    font-size: 12px;
    padding: 1px 4px;
    border-radius: 4px;
    background: var(--hover);
  }
</style>
