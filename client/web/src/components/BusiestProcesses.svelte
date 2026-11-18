<script lang="ts">
  import { api, type ProcessNetworkReport, type ProcessUsage } from '../lib/api';
  import { formatBytes, formatDateTime, formatPercent, formatRate } from '../lib/format';

  // the programs that used the most CPU, memory or network over the range
  let { host, from, to }: { host: string; from: number; to: number } = $props();

  const limit = 15;
  // the collector answers for up to a day, so a longer range shows its last day
  const maxSpan = 24 * 3600;
  const start = $derived(Math.max(from, to - maxSpan));

  let view = $state<'CPU' | 'Memory' | 'Network'>('CPU');
  let snapshots = $state(0);
  let firstTime = $state(0);
  let processes = $state<ProcessUsage[]>([]);
  let error = $state('');
  let loading = $state(false);

  // only the newest request may update the table
  let requestId = 0;

  $effect(() => {
    const request = { host, from: start, to, id: ++requestId };
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

  // network traffic comes from rollups, so it covers the whole range. A
  // collector or agent without it leaves the tab out.
  let network = $state<ProcessNetworkReport | null>(null);
  let networkId = 0;

  $effect(() => {
    const request = { host, from, to, id: ++networkId };
    api
      .processNetwork(request.host, request.from, request.to)
      .then((response) => {
        if (request.id === networkId) network = response;
      })
      .catch(() => {
        if (request.id === networkId) network = null;
      });
  });

  const hasNetwork = $derived(network !== null && network.seconds > 0);
  const shown = $derived(view === 'Network' && !hasNetwork ? 'CPU' : view);
  const networkRows = $derived(network?.programs.slice(0, limit) ?? []);

  const rows = $derived.by(() => {
    const average = shown === 'CPU' ? (p: ProcessUsage) => p.cpuAvg : (p: ProcessUsage) => p.memAvg;
    return [...processes].sort((a, b) => average(b) - average(a)).slice(0, limit);
  });
</script>

<div class="card busiest" class:refreshing={loading}>
  <div class="heading">
    <h3>Busiest over this range</h3>
    <div class="tabs" role="group" aria-label="Rank programs by">
      <button aria-pressed={shown === 'CPU'} class:selected={shown === 'CPU'} onclick={() => (view = 'CPU')}>By CPU</button>
      <button aria-pressed={shown === 'Memory'} class:selected={shown === 'Memory'} onclick={() => (view = 'Memory')}>By memory</button>
      {#if hasNetwork}
        <button aria-pressed={shown === 'Network'} class:selected={shown === 'Network'} onclick={() => (view = 'Network')}>
          By network
        </button>
      {/if}
    </div>
  </div>
  <p class="when muted">
    {#if shown === 'Network' && network}
      {formatBytes(network.rxBytes)} received and {formatBytes(network.txBytes)} sent by all programs, loopback left out.
    {:else if snapshots > 0}
      {start > from ? 'The last 24 hours of this range, from' : 'From'}
      {snapshots.toLocaleString()} snapshots since {formatDateTime(firstTime)}. Each keeps only the top 10 processes, so averages
      are a lower bound.
    {:else if !error && !loading}
      No process lists recorded in this range.
    {/if}
  </p>

  {#if shown === 'Network'}
    {#if networkRows.length > 0}
      <div class="table-scroll">
        <table class="data">
          <thead>
            <tr>
              <th>Program</th>
              <th class="right">Received</th>
              <th class="right">Sent</th>
              <th class="right" title="The fastest agent interval">Peak in</th>
              <th class="right" title="The fastest agent interval">Peak out</th>
            </tr>
          </thead>
          <tbody>
            {#each networkRows as program (program.name)}
              <tr>
                <td class="name" title={program.name}>{program.name}</td>
                <td class="right num">{formatBytes(program.rxBytes)}</td>
                <td class="right num">{formatBytes(program.txBytes)}</td>
                <td class="right num">{formatRate(program.rxPeak)}</td>
                <td class="right num">{formatRate(program.txPeak)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      <p class="secondary">No program used the network in this range.</p>
    {/if}
  {:else if error}
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
              <td class="right num">{formatPercent(shown === 'CPU' ? process.cpuAvg : process.memAvg, 1)}</td>
              <td class="right num">{formatPercent(shown === 'CPU' ? process.cpuPeak : process.memPeak, 1)}</td>
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
