<script lang="ts">
  import { api, type AlertRecord } from '../lib/api';
  import { appConfig } from '../lib/config.svelte';
  import { formatDateTime, formatDuration, formatNumber, formatPercent } from '../lib/format';
  import { poll } from '../lib/poll';
  import { hostPath, location, navigate } from '../lib/router.svelte';
  import StatTile from '../components/StatTile.svelte';
  import StatusBadge from '../components/StatusBadge.svelte';

  // filters live in the URL: ?show=all&host=web1&within=7d
  const within = [
    { key: '24h', label: 'Last 24 hours', seconds: 86400 },
    { key: '7d', label: 'Last 7 days', seconds: 7 * 86400 },
    { key: '30d', label: 'Last 30 days', seconds: 30 * 86400 },
    { key: 'all', label: 'All time', seconds: 0 },
  ];

  const showAll = $derived(location.query.get('show') === 'all');
  const hostFilter = $derived(location.query.get('host') ?? '');
  const withinKey = $derived(within.some((w) => w.key === location.query.get('within')) ? location.query.get('within')! : '7d');

  let alerts = $state<AlertRecord[]>([]);
  let hosts = $state<string[]>([]);
  let loaded = $state(false);
  let error = $state('');

  function setFilter(key: string, value: string) {
    const next = new URLSearchParams(location.query);
    if (value) next.set(key, value);
    else next.delete(key);
    const text = next.toString();
    navigate(location.path + (text ? `?${text}` : ''), true);
  }

  async function load(open: boolean, host: string, seconds: number) {
    try {
      const now = Math.floor(Date.now() / 1000);
      const response = await api.alerts({ open, host: host || undefined, from: seconds && !open ? now - seconds : undefined });
      alerts = response.alerts;
      error = '';
    } catch (e) {
      error = (e as Error).message;
    } finally {
      loaded = true;
    }
  }

  $effect(() => {
    const seconds = within.find((w) => w.key === withinKey)!.seconds;
    return poll(() => load(!showAll, hostFilter, seconds), appConfig.refreshSeconds);
  });

  $effect(() => {
    api
      .fleet()
      .then((fleet) => (hosts = fleet.hosts.map((h) => h.name)))
      .catch(() => {});
  });

  const openAlerts = $derived(alerts.filter((a) => !a.resolvedAt));
  const openCritical = $derived(openAlerts.filter((a) => a.severity === 2).length);
  const openWarning = $derived(openAlerts.filter((a) => a.severity === 1).length);

  function status(alert: AlertRecord): 'critical' | 'warning' | 'resolved' {
    if (alert.resolvedAt) return 'resolved';
    return alert.severity === 2 ? 'critical' : 'warning';
  }

  // what the alert watches, in words. target names the disk or service.
  function watched(alert: AlertRecord): string {
    switch (alert.metric) {
      case 'procUsage':
        return 'CPU';
      case 'memory':
        return 'Memory';
      case 'swap':
        return 'Swap';
      case 'ping':
        return 'Heartbeat';
      case 'disks':
      case 'services':
        return alert.target;
      default:
        return alert.metric;
    }
  }

  function value(alert: AlertRecord): string {
    switch (alert.metric) {
      case 'procUsage':
      case 'memory':
      case 'swap':
      case 'disks':
        return formatPercent(alert.value);
      case 'ping':
        return `silent ${formatDuration(alert.value)}`;
      case 'services':
        return alert.value ? 'Running' : 'Stopped';
      default:
        return formatNumber(alert.value);
    }
  }

  function duration(alert: AlertRecord): string {
    const end = alert.resolvedAt || Date.now() / 1000;
    return formatDuration(end - alert.startedAt);
  }
</script>

<div class="page">
  <header class="page-header">
    <h1>Alerts</h1>
  </header>

  <section class="tiles" aria-label="Open alerts">
    <StatTile label="Open critical" value={openCritical}>
      {#snippet extra()}
        {#if openCritical > 0}<StatusBadge status="critical" label="Needs attention" />{/if}
      {/snippet}
    </StatTile>
    <StatTile label="Open warning" value={openWarning} />
    {#if showAll}
      <StatTile label="Resolved" value={alerts.length - openAlerts.length} />
    {/if}
  </section>

  <div class="filters">
    <div class="segmented" role="group" aria-label="Which alerts">
      <button aria-pressed={!showAll} class:selected={!showAll} onclick={() => setFilter('show', '')}>Open</button>
      <button aria-pressed={showAll} class:selected={showAll} onclick={() => setFilter('show', 'all')}>All</button>
    </div>
    <label class="secondary">
      Host
      <select class="control" value={hostFilter} onchange={(e) => setFilter('host', e.currentTarget.value)}>
        <option value="">All hosts</option>
        {#each hosts as name (name)}<option value={name}>{name}</option>{/each}
      </select>
    </label>
    {#if showAll}
      <label class="secondary">
        Started
        <select class="control" value={withinKey} onchange={(e) => setFilter('within', e.currentTarget.value === '7d' ? '' : e.currentTarget.value)}>
          {#each within as w (w.key)}<option value={w.key}>{w.label}</option>{/each}
        </select>
      </label>
    {/if}
  </div>

  <div class="card list">
    {#if error}
      <p class="empty">Could not load alerts: {error}</p>
    {:else if !loaded}
      <p class="empty muted">Loading…</p>
    {:else if alerts.length === 0}
      <p class="empty secondary">
        {showAll ? 'No alerts in this time range.' : 'No open alerts.'}
      </p>
    {:else}
      <div class="table-scroll">
        <table class="data">
          <caption class="sr-only">Alerts</caption>
          <thead>
            <tr>
              <th>Status</th>
              <th>Host</th>
              <th>Rule</th>
              <th>Watching</th>
              <th class="right">Value</th>
              <th>Started</th>
              <th class="right">Duration</th>
            </tr>
          </thead>
          <tbody>
            {#each alerts as alert (alert.id)}
              <tr>
                <td>
                  <StatusBadge status={status(alert)} />
                  {#if alert.resolvedAt}<span class="muted severity">was {alert.severity === 2 ? 'critical' : 'warning'}</span>{/if}
                </td>
                <td><a href={hostPath(alert.host)}>{alert.host}</a></td>
                <td>{alert.rule}</td>
                <td class="secondary">{watched(alert)}</td>
                <td class="right num">{value(alert)}</td>
                <td class="num">{formatDateTime(alert.startedAt)}</td>
                <td class="right num">{duration(alert)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
</div>

<style>
  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 220px));
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

  .segmented {
    display: inline-flex;
    border: 1px solid var(--border);
    border-radius: 8px;
    overflow: hidden;
    background: var(--surface-raised);
  }

  .segmented button {
    border: none;
    background: none;
    padding: 6px 14px;
    color: var(--ink-secondary);
  }

  .segmented button.selected {
    color: var(--ink);
    font-weight: 600;
    background: var(--hover);
    box-shadow: inset 0 -2px 0 var(--accent);
  }

  .list {
    padding: 6px 4px;
  }

  .empty {
    margin: 0;
    padding: 32px;
    text-align: center;
  }

  .severity {
    font-size: 12px;
    margin-left: 4px;
  }
</style>
