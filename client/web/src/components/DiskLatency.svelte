<script lang="ts">
  import { align } from '../lib/align';
  import { api, type DiskLatencyReport } from '../lib/api';
  import { bucketLabel, bucketSpan, bucketTotals, heatmapRows, quantileSeries } from '../lib/disklatency';
  import { formatPercent } from '../lib/format';
  import { location, navigate } from '../lib/router.svelte';
  import ChartCard from './ChartCard.svelte';
  import Heatmap from './Heatmap.svelte';

  // How long a disk's reads or writes took, from the agent's eBPF tracer:
  // quantile lines, and a heatmap of where the IOs fell. found tells the
  // host page whether the host has a histogram in the range. The disk and
  // op live in the URL as ?disk= and ?op=.
  interface Props {
    host: string;
    from: number;
    to: number;
    onzoom?: (from: number, to: number) => void;
    found?: boolean;
  }
  let { host, from, to, onzoom, found = $bindable(false) }: Props = $props();

  let report = $state<DiskLatencyReport | null>(null);
  let error = $state('');
  let loading = $state(false);

  const device = $derived(location.query.get('disk') ?? '');
  const op = $derived<'read' | 'write'>(location.query.get('op') === 'write' ? 'write' : 'read');

  // only the newest request may update the panel
  let requestId = 0;

  $effect(() => {
    const request = { host, from, to, device, op, id: ++requestId };
    loading = true;
    api
      .diskLatency(request.host, request.from, request.to, request.device, request.op)
      .then((response) => {
        if (request.id !== requestId) return;
        report = response;
        found = response.devices.length > 0;
        error = '';
      })
      .catch((e) => {
        if (request.id !== requestId) return;
        error = (e as Error).message;
      })
      .finally(() => {
        if (request.id === requestId) loading = false;
      });
  });

  // the busiest disk and reads are the defaults, and stay out of the URL
  function pick(key: 'disk' | 'op', value: string) {
    const query = new URLSearchParams(location.query);
    if ((key === 'disk' && value === report?.devices[0]) || (key === 'op' && value === 'read')) query.delete(key);
    else query.set(key, value);
    const search = query.toString();
    navigate(search ? `${location.path}?${search}` : location.path, true);
  }

  const ios = $derived(op === 'write' ? 'writes' : 'reads');
  const quantiles = $derived(report ? align(quantileSeries(report)) : null);
  const heat = $derived(report ? heatmapRows(report) : null);
  const totals = $derived(report ? bucketTotals(report) : []);
  const starts = $derived(report?.bucketStarts ?? []);

  function detail(label: string, time: number): string {
    const count = heat?.counts.get(label)?.get(time) ?? 0;
    return `${count.toLocaleString()} ${ios}, ${bucketSpan(starts, Number(label))}`;
  }
</script>

{#if report && report.devices.length > 0}
  <div class="controls">
    <label>
      <span class="secondary">Disk</span>
      <select class="control" value={report.device} onchange={(e) => pick('disk', e.currentTarget.value)}>
        {#each report.devices as name (name)}<option value={name}>{name}</option>{/each}
      </select>
    </label>
    <div class="tabs" role="group" aria-label="Reads or writes">
      <button aria-pressed={op === 'read'} class:selected={op === 'read'} onclick={() => pick('op', 'read')}>Reads</button>
      <button aria-pressed={op === 'write'} class:selected={op === 'write'} onclick={() => pick('op', 'write')}>Writes</button>
    </div>
    {#if error}<span class="error">Could not load disk latency: {error}</span>{/if}
  </div>

  <div class="panels">
    <ChartCard
      title="Time per {op === 'write' ? 'write' : 'read'}, p50 to p99"
      unit="ms"
      note="Half of the {ios} took less than p50, all but 1% less than p99. Queueing included."
      data={quantiles}
      {loading}
      syncKey={host}
      {from}
      {to}
      {onzoom}
    />
    <ChartCard title="Where the {ios} fell" unit="percent" data={null} {loading} syncKey={host} note="Each column is 100% of that time's {ios}">
      {#snippet body()}
        {#if heat && heat.series.length > 0}
          <Heatmap
            series={heat.series}
            {from}
            {to}
            rowName={(label) => bucketLabel(starts, Number(label))}
            ariaLabel="Share of {ios} in each latency bucket over time"
            labelWidth={60}
            {detail}
          />
        {:else}
          <p class="secondary empty">No {ios} on {report?.device} in this range.</p>
        {/if}
      {/snippet}
      {#snippet table()}
        <table class="data">
          <thead><tr><th>Took</th><th class="right">{ios === 'writes' ? 'Writes' : 'Reads'}</th><th class="right">Share</th></tr></thead>
          <tbody>
            {#each totals as row (row.bucket)}
              <tr>
                <td>{bucketSpan(starts, row.bucket)}</td>
                <td class="right num">{row.count.toLocaleString()}</td>
                <td class="right num">{formatPercent(row.share)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/snippet}
    </ChartCard>
  </div>
{/if}

<style>
  .controls {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px;
    margin-bottom: 12px;
  }

  label {
    display: inline-flex;
    align-items: center;
    gap: 8px;
  }

  .tabs {
    display: inline-flex;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    overflow: hidden;
  }

  .tabs button {
    border: none;
    background: none;
    padding: 3px 10px;
    font-size: 12px;
    color: var(--ink-secondary);
  }

  .tabs button.selected {
    background: var(--hover);
    color: var(--ink);
    font-weight: 600;
  }

  .error {
    color: var(--critical-ink);
  }

  .empty {
    padding: 24px 0;
    text-align: center;
  }
</style>
