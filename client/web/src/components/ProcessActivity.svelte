<script lang="ts">
  import type { Snippet } from 'svelte';
  import { api, type ProcessActivityReport } from '../lib/api';
  import { formatAgo, formatDateTime, formatNumber, formatTime } from '../lib/format';
  import { describeExit, formatLifetime, formatUser, perMinute, sortGroups } from '../lib/proctrace';
  import Info from './Info.svelte';
  import StatTile from './StatTile.svelte';
  import StatusBadge from './StatusBadge.svelte';

  // What the agent's eBPF tracer saw over the chosen range: how many
  // processes started and ended, and the ones that ended badly. The host
  // page passes its charts, which sit between the totals and the exits.
  let { host, from, to, chartGrid }: { host: string; from: number; to: number; chartGrid?: Snippet } = $props();

  // rows a table shows before "Show all"
  const shortList = 15;

  let report = $state<ProcessActivityReport | null>(null);
  let error = $state('');
  let loading = $state(false);
  let view = $state<'groups' | 'latest'>('groups');
  let showAll = $state(false);
  let now = $state(Date.now() / 1000);

  // only the newest request may update the panel
  let requestId = 0;

  $effect(() => {
    const request = { host, from, to, id: ++requestId };
    loading = true;
    api
      .processActivity(request.host, request.from, request.to)
      .then((response) => {
        if (request.id !== requestId) return;
        report = response;
        now = Date.now() / 1000;
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

  const totals = $derived(report?.totals);
  const groups = $derived(sortGroups(report?.groups ?? []));
  const latest = $derived(report?.latest ?? []);
  const rows = $derived(view === 'groups' ? groups.length : latest.length);
  // exits are kept like raw data, so a long range lists them from later on
  const keptDays = $derived(report && report.exitsFrom > from ? Math.round((now - report.exitsFrom) / 86400) : 0);
  const span = $derived(to - from);

  const notes = $derived.by(() => {
    if (!totals) return [];
    const list: string[] = [];
    if (totals.failedExecs > 0) list.push(`${totals.failedExecs.toLocaleString()} execs failed, usually a shell trying each folder in PATH.`);
    if (totals.dropped > 0) list.push(`${totals.dropped.toLocaleString()} more ended badly but are not listed, more than 50 did in one interval.`);
    if (keptDays > 0) list.push(`Exits are kept for ${keptDays} days, so the OOM kills, crashes and lists below start ${formatDateTime(report?.exitsFrom ?? 0)}.`);
    return list;
  });

  function pick(next: 'groups' | 'latest') {
    view = next;
    showAll = false;
  }
</script>

<div class="activity" class:refreshing={loading}>
  {#if totals}
    <div class="tiles" aria-label="Process activity over this range">
      <StatTile label="Started" value={totals.execs.toLocaleString()}>
        {#snippet extra()}<span class="muted num">{formatNumber(perMinute(totals.execs, totals.seconds))} a minute</span>{/snippet}
      </StatTile>
      <StatTile label="Exited with an error" value={totals.nonZeroExits.toLocaleString()} />
      <StatTile label="Ended by a signal" value={totals.signaled.toLocaleString()} />
      <StatTile label="Out of memory kills" value={totals.oom.toLocaleString()}>
        {#snippet extra()}
          {#if totals.oom > 0}<StatusBadge status="critical" label="Memory ran out" />{/if}
        {/snippet}
      </StatTile>
      <StatTile label="Crashes" value={totals.crashes.toLocaleString()}>
        {#snippet extra()}
          {#if totals.crashes > 0}<StatusBadge status="critical" label="A program crashed" />{/if}
        {/snippet}
      </StatTile>
    </div>
    {#if notes.length > 0}
      <ul class="notes muted">
        {#each notes as note (note)}<li>{note}</li>{/each}
      </ul>
    {/if}
  {/if}

  {@render chartGrid?.()}

  <div class="card exits">
    <div class="heading">
      <div class="title-row">
        <h3>Processes that ended badly</h3>
        <Info text="By program lists the worst first: out of memory, then crashes, then kills. Latest lists the newest first." label="About exits" />
      </div>
      <div class="tabs" role="group" aria-label="List exits">
        <button aria-pressed={view === 'groups'} class:selected={view === 'groups'} onclick={() => pick('groups')}>By program</button>
        <button aria-pressed={view === 'latest'} class:selected={view === 'latest'} onclick={() => pick('latest')}>Latest</button>
      </div>
    </div>

    {#if error}
      <p class="secondary">Could not load process activity: {error}</p>
    {:else if report && report.totals.listed === 0}
      <p class="secondary">No process ended badly in this range.</p>
    {:else if view === 'groups'}
      {#if groups.length >= 100}<p class="when muted">Only the 100 worst kinds are listed.</p>{/if}
      <div class="table-scroll">
        <table class="data">
          <thead>
            <tr>
              <th>Program</th>
              <th>What happened</th>
              <th class="right">Times</th>
              <th class="right">Last</th>
              <th>Parent</th>
              <th class="right" title="Median time from start to exit">Lived</th>
            </tr>
          </thead>
          <tbody>
            {#each showAll ? groups : groups.slice(0, shortList) as group (`${group.name} ${group.status} ${group.signal} ${group.coreDumped} ${group.oom}`)}
              {@const meaning = describeExit(group)}
              <tr>
                <td class="name" title={group.name}>{group.name}</td>
                <td>
                  {#if meaning.badge}<StatusBadge status={meaning.badge} label={meaning.label} />{:else}<span class="secondary">{meaning.label}</span>{/if}
                </td>
                <td class="right num">{group.count.toLocaleString()}</td>
                <td class="right num" title={formatDateTime(group.last)}>{formatAgo(group.last, now)}</td>
                <td class="secondary">{group.parent || '–'}</td>
                <td class="right num">{formatLifetime(group.lifetimeSeconds)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      {#if report && report.totals.listed > latest.length}
        <p class="when muted">The newest {latest.length} of {report.totals.listed.toLocaleString()}.</p>
      {/if}
      <div class="table-scroll">
        <table class="data">
          <thead>
            <tr>
              <th>Time</th>
              <th>Program</th>
              <th>What happened</th>
              <th class="right">PID</th>
              <th>Parent</th>
              <th>User</th>
              <th class="right">Lived</th>
            </tr>
          </thead>
          <tbody>
            {#each showAll ? latest : latest.slice(0, shortList) as exit, i (`${exit.time} ${exit.pid} ${i}`)}
              {@const meaning = describeExit(exit)}
              <tr>
                <td class="num" title={formatDateTime(exit.time)}>{formatTime(exit.time, span)}</td>
                <td class="name" title={exit.name}>{exit.name}</td>
                <td>
                  {#if meaning.badge}<StatusBadge status={meaning.badge} label={meaning.label} />{:else}<span class="secondary">{meaning.label}</span>{/if}
                </td>
                <td class="right num">{exit.pid}</td>
                <td class="secondary">{exit.parent || '–'} <span class="muted num">({exit.ppid})</span></td>
                <td class="secondary">{formatUser(exit.uid)}</td>
                <td class="right num">{formatLifetime(exit.lifetimeSeconds)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}

    {#if !error && rows > shortList}
      <button class="more" onclick={() => (showAll = !showAll)}>{showAll ? 'Show fewer' : `Show all ${rows}`}</button>
    {/if}
  </div>
</div>

<style>
  .activity {
    display: flex;
    flex-direction: column;
    gap: 12px;
    transition: opacity 0.2s;
  }

  .refreshing {
    opacity: 0.6;
  }

  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: 12px;
  }

  .notes {
    margin: -4px 0 0;
    padding: 0;
    list-style: none;
    font-size: 12px;
  }

  .exits {
    padding: 14px;
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

  .name {
    max-width: 240px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  td {
    white-space: nowrap;
  }

  .more {
    margin-top: 8px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: none;
    padding: 4px 12px;
    font-size: 12px;
    color: var(--ink-secondary);
  }

  .more:hover {
    background: var(--hover);
  }
</style>
