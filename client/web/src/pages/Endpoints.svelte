<script lang="ts">
  import { untrack } from 'svelte';
  import { align, type Aligned } from '../lib/align';
  import { api, type EndpointStatus } from '../lib/api';
  import { appConfig } from '../lib/config.svelte';
  import { formatAgo, formatMs, formatPercent } from '../lib/format';
  import { poll } from '../lib/poll';
  import { location, navigate } from '../lib/router.svelte';
  import { rangeQuery, resolveRange } from '../lib/timerange';
  import ChartCard from '../components/ChartCard.svelte';
  import StatusBadge from '../components/StatusBadge.svelte';
  import TimeRangePicker from '../components/TimeRangePicker.svelte';

  // The HTTP checks the collector runs for endpoint alert rules. Response
  // time and the share of passing checks have different units, so each
  // endpoint gets one chart of each.
  type Metric = 'latency' | 'availability';

  let now = $state(Math.floor(Date.now() / 1000));
  const range = $derived(resolveRange(location.query, now));

  let endpoints = $state<EndpointStatus[]>([]);
  let loaded = $state(false);
  let error = $state('');
  let charts = $state<Record<string, { data: Aligned | null; loading: boolean; error: string }>>({});

  const chartKey = (name: string, metric: Metric) => `${name}|${metric}`;

  async function loadChart(name: string, metric: Metric, from: number, to: number) {
    const key = chartKey(name, metric);
    const previous = charts[key];
    charts[key] = { data: previous?.data ?? null, loading: true, error: '' };
    try {
      const response = await api.endpointSeries(name, metric, from, to);
      charts[key] = { data: align(response.series), loading: false, error: '' };
    } catch (e) {
      charts[key] = { data: previous?.data ?? null, loading: false, error: (e as Error).message };
    }
  }

  async function loadAll() {
    now = Math.floor(Date.now() / 1000);
    const { from, to } = untrack(() => range);
    try {
      endpoints = (await api.endpoints(from, to)).endpoints;
      error = '';
    } catch (e) {
      error = (e as Error).message;
      return;
    } finally {
      loaded = true;
    }
    for (const endpoint of endpoints) {
      loadChart(endpoint.name, 'latency', from, to);
      loadChart(endpoint.name, 'availability', from, to);
    }
  }

  // reload when the range in the URL changes. Live ranges also reload every
  // refresh interval, fixed ones load once.
  $effect(() => {
    const query = location.query.toString();
    const live = resolveRange(new URLSearchParams(query)).live;
    return untrack(() => {
      if (live) return poll(loadAll, appConfig.refreshSeconds);
      loadAll();
    });
  });

  function zoom(from: number, to: number) {
    navigate(location.path + rangeQuery(location.query, { from, to }));
  }

  // what the newest check got back
  function answer(endpoint: EndpointStatus): string {
    if (endpoint.statusCode) return `HTTP ${endpoint.statusCode} in ${formatMs(endpoint.latencyMs)}`;
    return 'No response';
  }
</script>

<div class="page">
  <header class="page-header">
    <h1>Endpoints</h1>
  </header>

  <div class="filters">
    <TimeRangePicker {range} />
    <span class="muted hint">Drag across a chart to zoom in</span>
  </div>

  {#if error}
    <p class="card message">Could not load endpoints: {error}</p>
  {:else if loaded && endpoints.length === 0}
    <div class="card message">
      <h2>No endpoint checks in this time range</h2>
      <p class="secondary">
        The collector checks the <code>endpoint</code> rules in <code>alerts.json</code> when it runs with
        <code>SYMON_ENABLE_ENDPOINT_MONITORING=true</code>.
      </p>
    </div>
  {/if}

  {#each endpoints as endpoint (endpoint.name)}
    <section class="endpoint">
      <div class="top">
        <div class="title">
          <h2>{endpoint.name}</h2>
          <p class="url muted">{endpoint.method} {endpoint.url}</p>
        </div>
        <StatusBadge status={endpoint.ok ? 'up' : 'down'} />
      </div>
      <p class="facts secondary">
        <span>{answer(endpoint)}, checked {formatAgo(endpoint.time)}</span>
        <span>{formatPercent(endpoint.uptimePct, endpoint.uptimePct < 100 ? 1 : 0)} of {endpoint.checks} checks passed</span>
        {#if endpoint.avgLatencyMs}<span>{formatMs(endpoint.avgLatencyMs)} on average</span>{/if}
      </p>
      {#if endpoint.error}<p class="error secondary">{endpoint.error}</p>{/if}

      <div class="grid">
        {#each [{ metric: 'latency', title: 'Response time', unit: 'ms' }, { metric: 'availability', title: 'Checks passing', unit: 'percent' }] as const as chart (chart.metric)}
          {@const state = charts[chartKey(endpoint.name, chart.metric)]}
          <ChartCard
            title={chart.title}
            unit={chart.unit}
            yMax={chart.metric === 'availability' ? 100 : undefined}
            area={chart.metric === 'latency'}
            note={chart.metric === 'latency' ? 'Until the response headers arrived' : undefined}
            data={state?.data ?? null}
            loading={state?.loading ?? true}
            error={state?.error}
            syncKey="endpoints"
            from={range.from}
            to={range.to}
            onzoom={zoom}
          />
        {/each}
      </div>
    </section>
  {/each}
</div>

<style>
  .filters {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px;
    padding: 10px 0;
    margin-bottom: 8px;
  }

  .hint {
    font-size: 12px;
  }

  .message {
    padding: 20px;
    margin: 0 0 16px;
  }

  .message p {
    margin: 6px 0 0;
  }

  .endpoint {
    margin-bottom: 28px;
  }

  .top {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 12px;
  }

  .title {
    min-width: 0;
  }

  h2 {
    font-size: 15px;
    margin: 0;
  }

  .url {
    margin: 2px 0 0;
    font-size: 12px;
    overflow-wrap: anywhere;
  }

  .facts {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 14px;
    margin: 6px 0 10px;
    font-size: 13px;
  }

  .error {
    margin: -4px 0 10px;
    font-size: 12px;
    overflow-wrap: anywhere;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 420px), 1fr));
    gap: 12px;
  }

  code {
    font-size: 12px;
    padding: 1px 4px;
    border-radius: 4px;
    background: var(--hover);
  }
</style>
