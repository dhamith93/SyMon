<script lang="ts">
  import { api, type ProcessUsage } from '../lib/api';
  import { formatDateTime, formatPercent } from '../lib/format';

  // the programs that used the most CPU or memory over the chosen range
  let { host, from, to }: { host: string; from: number; to: number } = $props();

  const limit = 15;

  let view = $state<'CPU' | 'Memory'>('CPU');
  let snapshots = $state(0);
  let firstTime = $state(0);
  let processes = $state<ProcessUsage[]>([]);
  let error = $state('');
  let loading = $state(false);

  // only the newest request may update the table
  let requestId = 0;

  $effect(() => {
    const request = { host, from, to, id: ++requestId };
    loading = true;
    api
      .processUsage(request.host, request.from, request.to)
      .then((response) => {
        if (request.id !== requestId) return;
        snapshots = response.snapshots;
        firstTime = response.firstTime;
        processes = response.processes;
        error = '';
      })
      .catch((e) => {
        if (request.id !== requestId) return;
        error = e.message;
        processes = [];
      })
      .finally(() => {
        if (request.id === requestId) loading = false;
      });
  });

  const rows = $derived.by(() => {
    const average = view === 'CPU' ? (p: ProcessUsage) => p.cpuAvg : (p: ProcessUsage) => p.memAvg;
    return [...processes].sort((a, b) => average(b) - average(a)).slice(0, limit);
  });
</script>

<div class="card busiest" class:refreshing={loading}>
  <div class="heading">
    <h3>Busiest over this range</h3>
    <div class="tabs" role="group" aria-label="Rank programs by">
      <button aria-pressed={view === 'CPU'} class:selected={view === 'CPU'} onclick={() => (view = 'CPU')}>By CPU</button>
      <button aria-pressed={view === 'Memory'} class:selected={view === 'Memory'} onclick={() => (view = 'Memory')}>By memory</button>
    </div>
  </div>
  <p class="when muted">
    {#if snapshots > 0}
      From {snapshots.toLocaleString()} snapshots since {formatDateTime(firstTime)}. Each keeps only the top 10 processes, so
      averages are a lower bound.
    {:else if !error && !loading}
      No process lists recorded in this range.
    {/if}
  </p>

  {#if error}
    <p class="secondary">{error}</p>
  {:else if rows.length > 0}
    <div class="table-scroll">
      <table class="data">
        <thead>
          <tr>
            <th>Program</th>
            <th class="right">Average</th>
            <th class="right">Peak</th>
            <th class="right">Seen</th>
          </tr>
        </thead>
        <tbody>
          {#each rows as process (process.name)}
            <tr>
              <td class="name" title={process.name}>{process.name}</td>
              <td class="right num">{formatPercent(view === 'CPU' ? process.cpuAvg : process.memAvg, 1)}</td>
              <td class="right num">{formatPercent(view === 'CPU' ? process.cpuPeak : process.memPeak, 1)}</td>
              <td class="right num" title="Share of snapshots with this program in the top lists">{formatPercent(process.seenPct)}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<style>
  .busiest {
    padding: 14px;
    transition: opacity 0.2s;
  }

  .refreshing {
    opacity: 0.6;
  }

  .heading {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  h3 {
    font-size: 14px;
  }

  .when {
    margin: 4px 0 8px;
    font-size: 12px;
  }

  .tabs {
    display: inline-flex;
    border: 1px solid var(--border);
    border-radius: 6px;
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

  .name {
    max-width: 280px;
    overflow: hidden;
    text-overflow: ellipsis;
  }
</style>
