<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { Aligned } from '../lib/align';
  import { formatDateTime, formatValue, type Unit } from '../lib/format';
  import TimeChart from './TimeChart.svelte';

  interface Props {
    title: string;
    unit: Unit;
    data: Aligned | null;
    loading?: boolean;
    error?: string;
    yMax?: number;
    syncKey: string;
    from?: number;
    to?: number;
    area?: boolean;
    // shown under the title, like what a click does
    note?: string;
    onzoom?: (from: number, to: number) => void;
    onpick?: (time: number) => void;
    // replaces the line chart, like the per core heatmap
    body?: Snippet;
    // rows for the table view when body replaces the chart
    table?: Snippet;
  }

  let { title, unit, data, loading = false, error = '', yMax, syncKey, from, to, area = false, note, onzoom, onpick, body, table }: Props = $props();

  let showTable = $state(false);
  const colors = ['--series-1', '--series-2', '--series-3', '--series-4', '--series-5', '--series-6', '--series-7', '--series-8'];
  const empty = $derived(!body && (!data || data.labels.length === 0 || data.times.length === 0));

  // newest first, real samples only
  const tableRows = $derived.by(() => {
    if (!data) return [];
    const rows = [];
    for (let i = data.times.length - 1; i >= 0; i--) {
      if (!Number.isInteger(data.times[i])) continue;
      rows.push({ time: data.times[i], values: data.values.map((series) => series[i]) });
    }
    return rows;
  });
</script>

<figure class="card" class:refreshing={loading && !empty}>
  <figcaption>
    <div class="heading">
      <h3>{title}</h3>
      {#if !empty || body}
        <button class="toggle" onclick={() => (showTable = !showTable)} aria-pressed={showTable}>
          {showTable ? 'Chart' : 'Table'}
        </button>
      {/if}
    </div>
    {#if note}<p class="note muted">{note}</p>{/if}
    {#if data && data.labels.length > 1 && !showTable}
      <ul class="legend">
        {#each data.labels as label, i (label)}
          <li><span class="key" style:background="var({colors[i]})"></span>{label}</li>
        {/each}
      </ul>
    {/if}
  </figcaption>

  {#if error}
    <p class="state">Could not load: {error}</p>
  {:else if loading && empty}
    <p class="state muted">Loading…</p>
  {:else if empty}
    <p class="state muted">No data in this time range.</p>
  {:else if showTable}
    <div class="table-view table-scroll">
      {#if body && table}
        {@render table()}
      {:else if data}
        <table class="data">
          <caption class="sr-only">{title}</caption>
          <thead>
            <tr>
              <th>Time</th>
              {#each data.labels as label (label)}<th class="right">{label}</th>{/each}
            </tr>
          </thead>
          <tbody>
            {#each tableRows as row (row.time)}
              <tr>
                <td class="num">{formatDateTime(row.time)}</td>
                {#each row.values as value, i (i)}
                  <td class="right num">{value == null ? '–' : formatValue(value, unit)}</td>
                {/each}
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </div>
  {:else if body}
    {@render body()}
  {:else if data}
    <TimeChart {data} {unit} {yMax} {syncKey} {from} {to} {area} {onzoom} {onpick} />
  {/if}

  {#if data && data.hidden.length > 0}
    <p class="note muted">Not shown: {data.hidden.join(', ')}. Charts show at most 8 series.</p>
  {/if}
</figure>

<style>
  figure {
    margin: 0;
    padding: 14px 14px 8px;
    min-width: 0;
    transition: opacity 0.2s;
  }

  .refreshing {
    opacity: 0.6;
  }

  .heading {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }

  h3 {
    font-size: 14px;
  }

  .toggle {
    border: 1px solid var(--border);
    background: none;
    border-radius: 6px;
    padding: 2px 8px;
    font-size: 12px;
    color: var(--ink-secondary);
  }

  .toggle:hover {
    background: var(--hover);
  }

  .note {
    margin: 2px 0 0;
    font-size: 12px;
  }

  .legend {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 14px;
    list-style: none;
    margin: 8px 0 4px;
    padding: 0;
    font-size: 12px;
    color: var(--ink-secondary);
  }

  .legend li {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .key {
    width: 14px;
    height: 2px;
    border-radius: 1px;
  }

  .state {
    margin: 0;
    padding: 48px 0;
    text-align: center;
  }

  .table-view {
    max-height: 260px;
    overflow-y: auto;
    margin-top: 8px;
  }
</style>
