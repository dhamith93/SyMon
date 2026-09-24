<script lang="ts">
  import type { Container } from '../lib/api';
  import { formatBytes, formatPercent, formatRate } from '../lib/format';

  // running containers from the latest snapshot, grouped by compose project
  let { containers }: { containers: Container[] } = $props();

  type SortKey = 'name' | 'cpu' | 'memory';
  let sortBy = $state<SortKey>('cpu');

  const name = (c: Container) => c.Name || c.ShortID;

  const groups = $derived.by(() => {
    const compare: Record<SortKey, (a: Container, b: Container) => number> = {
      name: (a, b) => name(a).localeCompare(name(b)),
      cpu: (a, b) => b.CPU.PercentOfHost - a.CPU.PercentOfHost,
      memory: (a, b) => b.Memory.Used - a.Memory.Used,
    };
    const byProject = new Map<string, Container[]>();
    for (const c of containers) {
      const project = c.ComposeProject || '';
      byProject.set(project, [...(byProject.get(project) ?? []), c]);
    }
    // named projects first, loose containers last
    return [...byProject.entries()]
      .sort(([a], [b]) => (a === '' ? 1 : b === '' ? -1 : a.localeCompare(b)))
      .map(([project, list]) => ({ project, list: list.sort(compare[sortBy]) }));
  });

  function network(c: Container): string {
    if (c.Network.SharesHostNetwork) return 'host network';
    if (!c.Network.Accessible) return 'unknown';
    if (c.Rates?.RxBytesPerSec == null || c.Rates.TxBytesPerSec == null) return '–';
    return `↓ ${formatRate(c.Rates.RxBytesPerSec)} ↑ ${formatRate(c.Rates.TxBytesPerSec)}`;
  }

  function memory(c: Container): string {
    const used = formatBytes(c.Memory.Used);
    return c.Memory.Limited ? `${used} of ${formatBytes(c.Memory.Limit)}` : used;
  }

  const anyNamed = $derived(containers.some((c) => c.MetadataAvailable));
</script>

<div class="card box">
  <div class="heading">
    <h3>Running containers</h3>
    <label class="secondary">
      Sort by
      <select class="control" bind:value={sortBy}>
        <option value="cpu">CPU</option>
        <option value="memory">Memory</option>
        <option value="name">Name</option>
      </select>
    </label>
  </div>
  {#if !anyNamed}
    <p class="muted note">Names and images need the agent to reach the Docker or Podman socket. Containers are shown by ID.</p>
  {/if}
  <div class="table-scroll">
    <table class="data">
      <caption class="sr-only">Running containers</caption>
      <thead>
        <tr>
          <th>Name</th>
          <th>Image</th>
          <th>State</th>
          <th class="right">CPU</th>
          <th class="right">Memory</th>
          <th class="right">Network</th>
          <th class="right">Disk IO</th>
          <th class="right">Pids</th>
        </tr>
      </thead>
      <tbody>
        {#each groups as group (group.project)}
          {#if group.project}
            <tr class="group"><th colspan="8">{group.project}</th></tr>
          {/if}
          {#each group.list as c (c.ID)}
            <tr>
              <td title={c.ID}>{name(c)}</td>
              <td class="secondary image" title={c.Image}>{c.Image || '–'}</td>
              <td class="secondary">{c.State}</td>
              <td class="right num">
                {formatPercent(c.CPU.PercentOfHost, 1)}
                {#if c.CPU.Limited}<span class="muted">· {formatPercent(c.CPU.PercentOfLimit)} of {c.CPU.AllocatedCores} cores</span>{/if}
              </td>
              <td class="right num">{memory(c)}{#if c.Memory.OOMKills > 0}<span class="oom"> · {c.Memory.OOMKills} OOM kills</span>{/if}</td>
              <td class="right num">{network(c)}</td>
              <td class="right num">{c.Rates ? `${formatRate(c.Rates.ReadBytesPerSec)} / ${formatRate(c.Rates.WriteBytesPerSec)}` : '–'}</td>
              <td class="right num">{c.Pids.Current}</td>
            </tr>
          {/each}
        {/each}
      </tbody>
    </table>
  </div>
</div>

<style>
  .box {
    padding: 14px;
    min-width: 0;
  }

  .heading {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    margin-bottom: 6px;
  }

  .heading label {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
  }

  h3 {
    font-size: 14px;
  }

  .note {
    margin: 0 0 6px;
    font-size: 12px;
  }

  .group th {
    padding-top: 12px;
    color: var(--ink-secondary);
    font-weight: 600;
  }

  .image {
    max-width: 240px;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .oom {
    color: var(--critical-ink);
  }
</style>
