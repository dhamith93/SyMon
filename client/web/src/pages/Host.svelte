<script lang="ts">
  import { untrack } from 'svelte';
  import type { Aligned, NamedSeries } from '../lib/align';
  import { api, type HostDetail } from '../lib/api';
  import { hostSections, loadChart, loadCores, type ChartSpec } from '../lib/charts';
  import { appConfig } from '../lib/config.svelte';
  import { formatAgo, formatBytes, formatDuration, formatMiB, formatPercent } from '../lib/format';
  import { poll } from '../lib/poll';
  import { hostPath, location, navigate } from '../lib/router.svelte';
  import { rangeQuery, resolveRange } from '../lib/timerange';
  import ChartCard from '../components/ChartCard.svelte';
  import ContainerTable from '../components/ContainerTable.svelte';
  import Heatmap from '../components/Heatmap.svelte';
  import ProcessTable from '../components/ProcessTable.svelte';
  import StatusBadge from '../components/StatusBadge.svelte';
  import TimeRangePicker from '../components/TimeRangePicker.svelte';

  let { host }: { host: string } = $props();

  // resolved again on every refresh, so live ranges move with the clock
  let now = $state(Math.floor(Date.now() / 1000));
  const range = $derived(resolveRange(location.query, now));

  let detail = $state<HostDetail | null>(null);
  let detailError = $state('');
  let charts = $state<Record<string, { data: Aligned | null; loading: boolean; error: string }>>({});
  let cores = $state<NamedSeries[]>([]);
  let coresLoading = $state(true);
  // the card stays hidden once we know the host sends no per core data
  const showCores = $derived(coresLoading || cores.length > 0);
  let hasCustomMetrics = $state(false);
  let processesAt = $state(0);
  let tick = $state(0);

  async function loadDetail() {
    try {
      detail = await api.host(host);
      detailError = '';
    } catch (e) {
      detailError = (e as Error).message;
    }
  }

  async function loadOne(spec: ChartSpec, from: number, to: number) {
    const previous = charts[spec.key];
    charts[spec.key] = { data: previous?.data ?? null, loading: true, error: '' };
    try {
      charts[spec.key] = { data: await loadChart(host, spec, from, to), loading: false, error: '' };
    } catch (e) {
      charts[spec.key] = { data: previous?.data ?? null, loading: false, error: (e as Error).message };
    }
  }

  async function loadCoreHeatmap(from: number, to: number) {
    coresLoading = true;
    try {
      cores = await loadCores(host, from, to);
    } catch {
      cores = [];
    } finally {
      coresLoading = false;
    }
  }

  function loadAll() {
    now = Math.floor(Date.now() / 1000);
    const { from, to } = untrack(() => range);
    tick++;
    loadDetail();
    for (const section of hostSections) {
      for (const spec of section.charts) loadOne(spec, from, to);
    }
    loadCoreHeatmap(from, to);
  }

  // reload when the host or the range in the URL changes. Live ranges
  // also reload every refresh interval, fixed ones load once.
  $effect(() => {
    void host;
    const query = location.query.toString();
    const live = resolveRange(new URLSearchParams(query)).live;
    return untrack(() => {
      if (live) return poll(loadAll, appConfig.refreshSeconds);
      loadAll();
    });
  });

  $effect(() => {
    api
      .customMetrics(host)
      .then((response) => (hasCustomMetrics = response.names.length > 0))
      .catch(() => (hasCustomMetrics = false));
  });

  function zoom(from: number, to: number) {
    navigate(location.path + rangeQuery(location.query, { from, to }));
  }

  function pickTime(time: number) {
    processesAt = time;
    document.getElementById('processes')?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }

  function visible(spec: ChartSpec): boolean {
    const chart = charts[spec.key];
    if (!spec.optional) return true;
    // optional charts appear once they have data
    return !!chart?.data && chart.data.labels.length > 0;
  }

  const snapshot = $derived(detail?.snapshot);
  const system = $derived(snapshot?.System);
  const cpu = $derived(snapshot?.ProcUsage);
</script>

<div class="page">
  <header class="page-header host-header">
    <div>
      <p class="crumbs muted"><a href="/">Hosts</a> /</p>
      <h1>{host}</h1>
      <p class="facts secondary">
        {#if detail}
          <StatusBadge status={detail.up ? 'up' : 'down'} />
          <span>{system?.OS} · {system?.Kernel}</span>
          {#if detail.up}<span>up {formatDuration(system?.UpTimeSeconds ?? 0)}</span>{:else}<span>last seen {formatAgo(detail.lastSeen)}</span>{/if}
          {#if cpu?.Model}<span>{cpu.Model}</span>{/if}
          {#if cpu?.NoOfCores}<span>{cpu.NoOfCores} CPUs</span>{/if}
          {#if snapshot?.Memory.Total}<span>{formatMiB(snapshot.Memory.Total)} memory</span>{/if}
        {:else if detailError}
          <span>{detailError === 'no data found' ? 'This host has not sent any data yet.' : `Could not load host: ${detailError}`}</span>
        {/if}
      </p>
    </div>
    {#if hasCustomMetrics}<a class="control link-button" href={hostPath(host, true)}>Custom metrics</a>{/if}
  </header>

  <div class="filters">
    <TimeRangePicker {range} />
    <span class="muted hint">Drag across a chart to zoom in</span>
  </div>

  {#each hostSections as section (section.title)}
    {@const specs = section.charts.filter(visible)}
    {#if specs.length > 0 || (section.title === 'CPU' && showCores)}
      <section>
        <h2>{section.title}</h2>
        <div class="grid">
          {#each specs as spec (spec.key)}
            {@const chart = charts[spec.key]}
            <ChartCard
              title={spec.title}
              unit={spec.unit}
              yMax={spec.yMax}
              area={spec.area}
              note={spec.note}
              data={chart?.data ?? null}
              loading={chart?.loading ?? true}
              error={chart?.error}
              syncKey={host}
              from={range.from}
              to={range.to}
              onzoom={zoom}
              onpick={pickTime}
            />
          {/each}
          {#if section.title === 'CPU' && showCores && cores.length === 0}
            <ChartCard title="CPU usage per core" unit="percent" data={null} loading syncKey={host} />
          {:else if section.title === 'CPU' && cores.length > 0}
            <ChartCard title="CPU usage per core" unit="percent" data={null} loading={coresLoading} syncKey={host}>
              {#snippet body()}
                <Heatmap series={cores} from={range.from} to={range.to} />
              {/snippet}
              {#snippet table()}
                <table class="data">
                  <thead><tr><th>CPU</th><th class="right">Average</th><th class="right">Peak</th></tr></thead>
                  <tbody>
                    {#each cores as core (core.label)}
                      {@const values = core.points.map((p) => p[1])}
                      <tr>
                        <td>cpu {core.label}</td>
                        <td class="right num">{formatPercent(values.reduce((a, b) => a + b, 0) / Math.max(1, values.length))}</td>
                        <td class="right num">{formatPercent(Math.max(0, ...values))}</td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              {/snippet}
            </ChartCard>
          {/if}
        </div>
      </section>
    {/if}
  {/each}

  {#if (snapshot?.Containers ?? []).length > 0}
    <section>
      <h2>Containers right now</h2>
      <ContainerTable containers={snapshot?.Containers ?? []} />
    </section>
  {/if}

  <section id="processes">
    <h2>Processes</h2>
    <ProcessTable {host} at={processesAt} {tick} onlatest={() => (processesAt = 0)} />
  </section>

  {#if snapshot}
    <section>
      <h2>Right now</h2>
      <div class="grid tables">
        <div class="card box">
          <h3>Disks</h3>
          <div class="table-scroll">
            <table class="data">
              <thead><tr><th>Mount</th><th>Device</th><th>Type</th><th class="right">Used</th><th class="right">Size</th><th class="right">Inodes</th></tr></thead>
              <tbody>
                {#each snapshot.Disk ?? [] as disk (disk.FileSystem + disk.MountedOn)}
                  <tr>
                    <td>{disk.MountedOn}</td>
                    <td class="secondary">{disk.FileSystem}</td>
                    <td class="secondary">{disk.Type}</td>
                    <td class="right num">{disk.Usage.Usage}</td>
                    <td class="right num">{formatBytes(disk.Usage.Size)}</td>
                    <td class="right num">{disk.Inodes.Usage}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        </div>

        <div class="card box">
          <h3>Network interfaces</h3>
          <div class="table-scroll">
            <table class="data">
              <thead><tr><th>Interface</th><th>State</th><th>Address</th><th class="right">Received</th><th class="right">Sent</th></tr></thead>
              <tbody>
                {#each snapshot.Networks ?? [] as network (network.Interface)}
                  <tr>
                    <td>{network.Interface}</td>
                    <td class="secondary">{network.Usage.State || '–'}</td>
                    <td class="secondary">{network.Ip || network.Ipv6 || '–'}</td>
                    <td class="right num">{formatBytes(network.Usage.RxBytes)}</td>
                    <td class="right num">{formatBytes(network.Usage.TxBytes)}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        </div>

        {#if (snapshot.Services ?? []).length > 0}
          <div class="card box">
            <h3>Services</h3>
            <table class="data">
              <tbody>
                {#each snapshot.Services ?? [] as service (service.Name)}
                  <tr>
                    <td>{service.Name}</td>
                    <td class="right"><StatusBadge status={service.Running ? 'up' : 'down'} label={service.Running ? 'Running' : 'Stopped'} /></td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}

        {#if (system?.LoggedInUsers ?? []).length > 0}
          <div class="card box">
            <h3>Logged in users</h3>
            <table class="data">
              <tbody>
                {#each system?.LoggedInUsers ?? [] as user (user.Username + user.LoggedInTime)}
                  <tr>
                    <td>{user.Username}</td>
                    <td class="secondary">{user.RemoteHost || 'local'}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </div>
    </section>
  {/if}
</div>

<style>
  .host-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-end;
    gap: 16px;
    flex-wrap: wrap;
  }

  .crumbs {
    margin: 0 0 2px;
    font-size: 12px;
  }

  .facts {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px 14px;
    margin: 6px 0 0;
    font-size: 13px;
  }

  .link-button {
    display: inline-flex;
    align-items: center;
    text-decoration: none;
    color: var(--ink);
  }

  .link-button:hover {
    text-decoration: none;
  }

  .filters {
    position: sticky;
    top: 0;
    z-index: 20;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px;
    padding: 10px 0;
    margin-bottom: 8px;
    background: var(--page);
  }

  .hint {
    font-size: 12px;
  }

  section {
    margin-bottom: 24px;
  }

  h2 {
    font-size: 15px;
    margin-bottom: 10px;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 420px), 1fr));
    gap: 12px;
  }

  .box {
    padding: 14px;
    min-width: 0;
  }

  .box h3 {
    font-size: 14px;
    margin-bottom: 6px;
  }
</style>
