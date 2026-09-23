<script lang="ts">
  import { api, type Process } from '../lib/api';
  import { formatDateTime, formatPercent } from '../lib/format';

  // top processes at `at`, or the latest when at is 0. A new tick reloads.
  let { host, at, tick, onlatest }: { host: string; at: number; tick: number; onlatest: () => void } = $props();

  let view = $state<'CPU' | 'Memory'>('CPU');
  let snapshotTime = $state(0);
  let processes = $state<{ CPU: Process[]; Memory: Process[] }>({ CPU: [], Memory: [] });
  let error = $state('');
  let loading = $state(false);

  // only the newest request may update the table
  let requestId = 0;

  $effect(() => {
    const request = { host, at, tick, id: ++requestId };
    loading = true;
    api
      .processes(request.host, request.at || undefined)
      .then((response) => {
        if (request.id !== requestId) return;
        snapshotTime = response.time;
        processes = { CPU: response.processes.CPU ?? [], Memory: response.processes.Memory ?? [] };
        error = '';
      })
      .catch((e) => {
        if (request.id !== requestId) return;
        error = e.status === 404 ? 'No process list recorded at or before this time.' : e.message;
        processes = { CPU: [], Memory: [] };
      })
      .finally(() => (loading = false));
  });
</script>

<div class="card processes" class:refreshing={loading}>
  <div class="heading">
    <h3>Top processes</h3>
    <div class="tabs" role="group" aria-label="Sort processes by">
      <button aria-pressed={view === 'CPU'} class:selected={view === 'CPU'} onclick={() => (view = 'CPU')}>By CPU</button>
      <button aria-pressed={view === 'Memory'} class:selected={view === 'Memory'} onclick={() => (view = 'Memory')}>By memory</button>
    </div>
  </div>
  <p class="when muted">
    {#if at}
      At {formatDateTime(snapshotTime || at)} · <button class="link" onclick={onlatest}>Show latest</button>
    {:else}
      Latest{snapshotTime ? `, ${formatDateTime(snapshotTime)}` : ''} · click a point on any chart to see the processes at that time
    {/if}
  </p>

  {#if error}
    <p class="secondary">{error}</p>
  {:else}
    <div class="table-scroll">
      <table class="data">
        <thead>
          <tr>
            <th>PID</th>
            <th>Name</th>
            <th>User</th>
            <th class="right">CPU</th>
            <th class="right">Memory</th>
            <th class="right">Threads</th>
            <th>State</th>
          </tr>
        </thead>
        <tbody>
          {#each processes[view] as process (process.Pid)}
            <tr>
              <td class="num">{process.Pid}</td>
              <td class="name" title={process.ExecPath}>{process.Name || process.ExecPath}</td>
              <td>{process.User}</td>
              <td class="right num">{formatPercent(process.CPUUsage, 1)}</td>
              <td class="right num">{formatPercent(process.MemUsage, 1)}</td>
              <td class="right num">{process.Threads || '–'}</td>
              <td class="secondary">{process.State || '–'}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<style>
  .processes {
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

  .link {
    border: none;
    background: none;
    padding: 0;
    color: var(--accent-ink);
    font-size: 12px;
  }

  .link:hover {
    text-decoration: underline;
  }

  .name {
    max-width: 280px;
    overflow: hidden;
    text-overflow: ellipsis;
  }
</style>
