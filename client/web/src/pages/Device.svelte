<script lang="ts">
  import { untrack } from 'svelte';
  import { align, maxSeries, type Aligned } from '../lib/align';
  import { api, ApiError, type DeviceDetail, type SeriesData } from '../lib/api';
  import { isAdmin } from '../lib/auth.svelte';
  import { appConfig } from '../lib/config.svelte';
  import {
    defaultPorts,
    deviceStatus,
    formatSpeed,
    kindLabel,
    portState,
    readPorts,
    sections,
    togglePort,
    valuesOf,
    writePorts,
    type ChartSpec,
  } from '../lib/devices';
  import { formatAgo, formatBits, formatDateTime, formatDuration, formatNumber, formatPercent } from '../lib/format';
  import { poll } from '../lib/poll';
  import { hostPath, location, navigate } from '../lib/router.svelte';
  import { rangeQuery, resolveRange } from '../lib/timerange';
  import ChartCard from '../components/ChartCard.svelte';
  import StatTile from '../components/StatTile.svelte';
  import StatusBadge from '../components/StatusBadge.svelte';
  import TimeRangePicker from '../components/TimeRangePicker.svelte';

  // One SNMP device: what its last poll read, and charts by profile. The
  // picked ports live in the URL, one ?port= per palette slot.
  let { device }: { device: string } = $props();

  let now = $state(Math.floor(Date.now() / 1000));
  const range = $derived(resolveRange(location.query, now));

  let detail = $state<DeviceDetail | null>(null);
  let loaded = $state(false);
  let error = $state('');
  let notFound = $state(false);
  let charts = $state<Record<string, { data: Aligned | null; loading: boolean; error: string }>>({});
  // the busiest ports at the first load, kept so refreshes do not repaint
  let firstPorts = $state<string[] | null>(null);
  let allPorts = $state(false);
  let confirmRemove = $state(false);
  let removeError = $state('');

  const summary = $derived(detail?.summary);
  const report = $derived(detail?.state);
  const read = $derived(!!report && report.Time > 0);
  const badge = $derived(summary ? deviceStatus(summary) : null);
  const picked = $derived(readPorts(location.query) ?? firstPorts ?? []);
  const pageSections = $derived(report && read ? sections(report, picked) : []);
  const portList = $derived(report?.Interfaces ?? []);
  const anyUp = $derived(portList.some((p) => p.Up));
  const shownPorts = $derived(allPorts || !anyUp ? portList : portList.filter((p) => p.Up || picked.includes(p.Name)));
  const rx = $derived(report ? valuesOf(report, 'if_rx_bps') : new Map<string, number>());
  const tx = $derived(report ? valuesOf(report, 'if_tx_bps') : new Map<string, number>());
  const rxErrors = $derived(report ? valuesOf(report, 'if_rx_errors_ps') : new Map<string, number>());
  const txErrors = $derived(report ? valuesOf(report, 'if_tx_errors_ps') : new Map<string, number>());
  const full = $derived(picked.filter((p) => p !== '').length >= maxSeries);

  // error charts only earn their space when a picked port had errors
  const isErrors = (spec: ChartSpec) => spec.key === 'if_rx_errors_ps' || spec.key === 'if_tx_errors_ps';
  const hadErrors = (key: string) => (charts[key]?.data?.values ?? []).some((series) => series.some((v) => typeof v === 'number' && v > 0));

  async function loadChart(spec: ChartSpec, from: number, to: number) {
    const previous = charts[spec.key];
    charts[spec.key] = { data: previous?.data ?? null, loading: true, error: '' };
    try {
      if (spec.labels && spec.labels.length === 0) {
        charts[spec.key] = { data: null, loading: false, error: '' };
        return;
      }
      const responses = await Promise.all(spec.parts.map((part) => api.deviceSeries(device, part.metric, from, to, spec.labels ?? [])));
      let series: SeriesData[] = responses.flatMap((response, i) =>
        response.series.map((s) => ({ ...s, label: s.label || spec.parts[i].name || spec.title })),
      );
      if (spec.labels) {
        const order = spec.labels;
        series = series.filter((s) => order.includes(s.label)).sort((a, b) => order.indexOf(a.label) - order.indexOf(b.label));
      }
      const data = align(series);
      if (spec.labels && spec.colors) data.colors = data.labels.map((label) => spec.colors![spec.labels!.indexOf(label)]);
      charts[spec.key] = { data, loading: false, error: '' };
    } catch (e) {
      charts[spec.key] = { data: previous?.data ?? null, loading: false, error: (e as Error).message };
    }
  }

  function loadCharts() {
    const { from, to } = untrack(() => range);
    for (const section of untrack(() => pageSections)) for (const spec of section.charts) loadChart(spec, from, to);
  }

  async function loadAll() {
    now = Math.floor(Date.now() / 1000);
    try {
      detail = await api.device(device);
      error = '';
      notFound = false;
      if (firstPorts === null && detail.state.Time > 0) firstPorts = defaultPorts(detail.state);
      loadCharts();
    } catch (e) {
      error = (e as Error).message;
      notFound = e instanceof ApiError && e.status === 404;
    } finally {
      loaded = true;
    }
  }

  // reload when the range or the picked ports change. Live ranges also
  // reload every refresh interval, fixed ones load once.
  $effect(() => {
    const query = location.query.toString();
    const live = resolveRange(new URLSearchParams(query)).live;
    return untrack(() => {
      if (live) return poll(loadAll, appConfig.refreshSeconds);
      loadAll();
    });
  });

  function toggle(port: string) {
    const query = new URLSearchParams(location.query);
    writePorts(query, togglePort(picked, port));
    navigate(`${location.path}?${query}`, true);
  }

  function zoom(from: number, to: number) {
    navigate(location.path + rangeQuery(location.query, { from, to }));
  }

  async function remove() {
    try {
      await api.removeDevice(device);
      appConfig.devices = appConfig.devices.filter((d) => d !== device);
      navigate('/devices');
    } catch (e) {
      removeError = (e as Error).message;
    }
  }

  const slotColor = (port: string) => `var(--series-${picked.indexOf(port) + 1})`;
</script>

<div class="page">
  <header class="page-header">
    <p class="crumbs muted"><a href="/devices">Devices</a> /</p>
    <div class="title">
      <h1>{device}</h1>
      {#if badge}<StatusBadge status={badge.status} label={badge.label} />{/if}
    </div>
    {#if summary}
      <p class="secondary about">
        <span>{kindLabel(summary.kind)} at {summary.address}</span>
        {#if read && !summary.error && report?.System.UptimeSeconds}<span>up {formatDuration(report.System.UptimeSeconds)}</span>{/if}
        <span>
          polled by
          {#if summary.host}<a href={hostPath(summary.host)}>{summary.host}</a>{:else}a host that was removed{/if}
          every {formatDuration(summary.intervalSeconds)}, last {formatAgo(summary.reportedAt, now)}
        </span>
      </p>
      {#if report?.System.Descr}<p class="descr muted" title={report.System.Descr}>{report.System.Descr}</p>{/if}
    {/if}
  </header>

  {#if error}
    <p class="card message">
      {#if notFound}There is no device named {device}. <a href="/devices">See every device</a>{:else}Could not load {device}: {error}{/if}
    </p>
  {/if}

  {#if summary && report}
    {#if summary.stale}
      <p class="card notice" role="alert">
        <StatusBadge status="warning" label="No new reports" />
        <span>Nothing since {formatDateTime(summary.reportedAt)}, so alerts wait. Is the agent on {summary.host || 'its host'} running?</span>
      </p>
    {:else if summary.error}
      <p class="card notice" role="alert">
        <StatusBadge status="critical" label="Unreachable" />
        <span>
          The last poll said: {summary.error}.
          {#if read}What follows is from {formatDateTime(report.Time)}, the last poll that could read it.{/if}
        </span>
      </p>
    {/if}

    {#if !read}
      <div class="card message">
        <h2>Nothing read yet</h2>
        <p class="secondary">The agent has not been able to read this device. Check the address, the community and that SNMP v2c is on.</p>
      </div>
    {:else}
      <section class="tiles" aria-label="Device summary">
        {#if summary.upsOnBattery !== null}
          <StatTile label="Power" value={summary.upsOnBattery ? 'Battery' : 'Line'}>
            {#snippet extra()}
              {#if summary.upsOnBattery}<StatusBadge status="warning" label="On battery" />{:else}<StatusBadge status="up" label="On line power" />{/if}
            {/snippet}
          </StatTile>
        {/if}
        {#if summary.upsChargePct !== null}<StatTile label="Battery charge" value={formatPercent(summary.upsChargePct)} />{/if}
        {#if summary.upsMinutesLeft !== null}<StatTile label="Minutes left" value={Math.round(summary.upsMinutesLeft)} />{/if}
        {#if summary.upsLoadPct !== null}<StatTile label="Load" value={formatPercent(summary.upsLoadPct)} />{/if}
        {#if summary.cpuPct !== null}<StatTile label="CPU" value={formatPercent(summary.cpuPct)} />{/if}
        {#if summary.memUsedPct !== null}<StatTile label="Memory used" value={formatPercent(summary.memUsedPct)} />{/if}
        {#if summary.ports > 0}
          <StatTile label="Ports up" value="{summary.portsUp} / {summary.ports}" />
          <StatTile label="In, all ports" value={formatBits(summary.rxBps)} />
          <StatTile label="Out, all ports" value={formatBits(summary.txBps)} />
        {/if}
      </section>

      <div class="filters">
        <TimeRangePicker {range} />
      </div>

      {#each pageSections as section (section.key)}
        <section>
          <h2>{section.title}</h2>
          {#if section.key === 'ports'}
            <div class="card box">
              <div class="heading">
                <p class="muted hint">
                  Tick up to {maxSeries} ports to chart.{#if full}
                    All {maxSeries} colors are taken, untick one first.{/if}
                </p>
                {#if anyUp && portList.some((p) => !p.Up)}
                  <label class="check secondary">
                    <input type="checkbox" bind:checked={allPorts} />
                    Show ports without a link
                  </label>
                {/if}
              </div>
              <div class="table-scroll">
                <table class="data">
                  <caption class="sr-only">Ports</caption>
                  <thead>
                    <tr>
                      <th><span class="sr-only">Chart</span></th>
                      <th>Port</th>
                      <th>Status</th>
                      <th class="right">Speed</th>
                      <th class="right">In</th>
                      <th class="right">Out</th>
                      <th class="right">Errors</th>
                    </tr>
                  </thead>
                  <tbody>
                    {#each shownPorts as port (port.Name)}
                      {@const portBadge = portState(port)}
                      {@const on = picked.includes(port.Name)}
                      {@const errors = (rxErrors.get(port.Name) ?? 0) + (txErrors.get(port.Name) ?? 0)}
                      <tr>
                        <td class="tick">
                          <label>
                            <input type="checkbox" checked={on} disabled={!on && full} onchange={() => toggle(port.Name)} />
                            <span class="sr-only">Chart {port.Name}</span>
                            {#if on}<span class="key" style:background={slotColor(port.Name)} aria-hidden="true"></span>{/if}
                          </label>
                        </td>
                        <td>{port.Name}{#if port.Alias}<span class="muted alias">{port.Alias}</span>{/if}</td>
                        <td>
                          <!-- an unreachable device's ports are only what they were -->
                          {#if typeof portBadge === 'string'}<span class="muted">{portBadge}</span>
                          {:else if summary.error}<span class="muted">Was up</span>
                          {:else}<StatusBadge status={portBadge.status} label={portBadge.label} />{/if}
                        </td>
                        <td class="right num">{formatSpeed(port.SpeedBps)}</td>
                        <td class="right num">{rx.has(port.Name) ? formatBits(rx.get(port.Name) ?? 0) : '–'}</td>
                        <td class="right num">{tx.has(port.Name) ? formatBits(tx.get(port.Name) ?? 0) : '–'}</td>
                        <td class="right num">{rxErrors.has(port.Name) || txErrors.has(port.Name) ? `${formatNumber(errors)}/s` : '–'}</td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
            </div>
          {/if}

          <div class="grid">
            {#each section.charts as spec (spec.key)}
              {#if !isErrors(spec) || hadErrors(spec.key)}
                {@const chart = charts[spec.key]}
                <ChartCard
                  title={spec.title}
                  unit={spec.unit}
                  yMax={spec.yMax}
                  note={spec.labels?.length === 0 ? 'Tick a port above to chart it' : isErrors(spec) ? undefined : spec.note}
                  area={spec.parts.length === 1 && !spec.labels}
                  data={chart?.data ?? null}
                  loading={chart?.loading ?? true}
                  error={chart?.error}
                  syncKey="device-{device}"
                  from={range.from}
                  to={range.to}
                  onzoom={zoom}
                />
              {/if}
            {/each}
          </div>
          {#if section.key === 'ports' && picked.some((p) => p !== '') && !hadErrors('if_rx_errors_ps') && !hadErrors('if_tx_errors_ps')}
            <p class="muted hint">No errors on the charted ports in this range.</p>
          {/if}
        </section>
      {/each}
    {/if}

    {#if isAdmin()}
      <section class="remove">
        {#if confirmRemove}
          <span class="secondary">Remove {device}? Its charts stay until retention drops them, and it comes back with the next report.</span>
          <button class="control danger" onclick={remove}>Remove</button>
          <button class="control" onclick={() => (confirmRemove = false)}>Keep</button>
        {:else}
          <span class="muted">Moving the poller to another host? Remove the device first, only its host may report it.</span>
          <button class="control" onclick={() => (confirmRemove = true)}>Remove device</button>
        {/if}
        {#if removeError}<p class="error" role="alert">{removeError}</p>{/if}
      </section>
    {/if}
  {/if}
</div>

<style>
  .crumbs {
    margin: 0 0 2px;
    font-size: 12px;
  }

  .title {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px 12px;
  }

  .about {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 14px;
    margin: 4px 0 0;
  }

  .descr {
    margin: 4px 0 0;
    font-size: 12px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .message {
    padding: 20px;
    margin: 0 0 16px;
  }

  .message p {
    margin: 6px 0 0;
  }

  .notice {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px 10px;
    padding: 12px 14px;
    margin: 0 0 16px;
    overflow-wrap: anywhere;
  }

  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: 12px;
    margin-bottom: 16px;
  }

  .filters {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px;
    padding: 10px 0;
    margin-bottom: 8px;
  }

  section {
    margin-bottom: 24px;
    min-width: 0;
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
    margin-bottom: 12px;
    min-width: 0;
  }

  .heading {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    align-items: baseline;
    gap: 6px 12px;
    margin-bottom: 8px;
  }

  .hint {
    margin: 0;
    font-size: 12px;
  }

  section > .hint {
    margin-top: 8px;
  }

  .check {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
  }

  .tick {
    width: 1%;
    white-space: nowrap;
  }

  .tick label {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .key {
    display: inline-block;
    width: 10px;
    height: 10px;
    border-radius: 2px;
  }

  .alias {
    margin-left: 8px;
  }

  .remove {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px 12px;
    font-size: 13px;
  }

  .error {
    flex-basis: 100%;
    margin: 0;
    color: var(--critical);
  }
</style>
