<script lang="ts">
  import { untrack } from 'svelte';
  import { align, type Aligned } from '../lib/align';
  import { api } from '../lib/api';
  import { appConfig } from '../lib/config.svelte';
  import { poll } from '../lib/poll';
  import { hostPath, location, navigate } from '../lib/router.svelte';
  import { rangeQuery, resolveRange } from '../lib/timerange';
  import ChartCard from '../components/ChartCard.svelte';
  import TimeRangePicker from '../components/TimeRangePicker.svelte';

  // Custom metrics can have any unit, so each gets its own chart, all
  // sharing one cursor, instead of different units on one axis.
  let { host }: { host: string } = $props();

  let now = $state(Math.floor(Date.now() / 1000));
  const range = $derived(resolveRange(location.query, now));

  let names = $state<string[]>([]);
  let loaded = $state(false);
  let charts = $state<Record<string, { data: Aligned | null; loading: boolean; error: string }>>({});

  // the picked metrics live in the URL as ?m=a,b
  const picked = $derived.by(() => {
    const fromUrl = (location.query.get('m') ?? '').split(',').filter((name) => names.includes(name));
    return fromUrl.length > 0 || location.query.has('m') ? fromUrl : names.slice(0, 4);
  });

  function toggle(name: string) {
    const next = picked.includes(name) ? picked.filter((n) => n !== name) : [...picked, name];
    const query = new URLSearchParams(location.query);
    query.set('m', next.join(','));
    navigate(`${location.path}?${query}`, true);
  }

  async function loadOne(name: string, from: number, to: number) {
    const previous = charts[name];
    charts[name] = { data: previous?.data ?? null, loading: true, error: '' };
    try {
      const response = await api.series(host, 'custom', from, to, { label: name });
      charts[name] = { data: align(response.series), loading: false, error: '' };
    } catch (e) {
      charts[name] = { data: previous?.data ?? null, loading: false, error: (e as Error).message };
    }
  }

  function loadAll() {
    now = Math.floor(Date.now() / 1000);
    const { from, to } = untrack(() => range);
    for (const name of untrack(() => picked)) loadOne(name, from, to);
  }

  $effect(() => {
    api
      .customMetrics(host)
      .then((response) => (names = response.names))
      .catch(() => (names = []))
      .finally(() => (loaded = true));
  });

  $effect(() => {
    void host;
    void picked.join(',');
    const live = resolveRange(new URLSearchParams(location.query.toString())).live;
    return untrack(() => {
      if (live) return poll(loadAll, appConfig.refreshSeconds);
      loadAll();
    });
  });

  function zoom(from: number, to: number) {
    navigate(location.path + rangeQuery(location.query, { from, to }));
  }
</script>

<div class="page">
  <header class="page-header">
    <p class="crumbs muted"><a href="/">Hosts</a> / <a href={hostPath(host)}>{host}</a> /</p>
    <h1>Custom metrics</h1>
  </header>

  <div class="filters">
    <TimeRangePicker {range} />
  </div>

  {#if loaded && names.length === 0}
    <div class="card empty">
      <p>This host has not sent any custom metrics.</p>
      <p class="secondary">Send one with <code>agent -custom -name=queue -unit=jobs -value=12</code>.</p>
    </div>
  {:else}
    <fieldset class="picker">
      <legend class="secondary">Metrics</legend>
      {#each names as name (name)}
        <label class="chip" class:selected={picked.includes(name)}>
          <input type="checkbox" checked={picked.includes(name)} onchange={() => toggle(name)} />
          {name}
        </label>
      {/each}
    </fieldset>

    <div class="grid">
      {#each picked as name (name)}
        {@const chart = charts[name]}
        <ChartCard
          title={name}
          unit="number"
          area
          data={chart?.data ?? null}
          loading={chart?.loading ?? true}
          error={chart?.error}
          syncKey="custom-{host}"
          from={range.from}
          to={range.to}
          onzoom={zoom}
        />
      {/each}
    </div>
  {/if}
</div>

<style>
  .crumbs {
    margin: 0 0 2px;
    font-size: 12px;
  }

  .filters {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    margin-bottom: 12px;
  }

  .picker {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    border: none;
    padding: 0;
    margin: 0 0 16px;
  }

  .picker legend {
    padding: 0;
    margin-bottom: 6px;
    font-size: 12px;
  }

  .chip {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 4px 10px;
    border: 1px solid var(--border);
    border-radius: 999px;
    background: var(--surface-raised);
    cursor: pointer;
  }

  .chip.selected {
    border-color: var(--accent);
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 420px), 1fr));
    gap: 12px;
  }

  .empty {
    padding: 20px;
  }

  .empty p {
    margin: 0 0 6px;
  }

  code {
    font-size: 12px;
    padding: 1px 4px;
    border-radius: 4px;
    background: var(--hover);
  }
</style>
