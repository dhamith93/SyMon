<script lang="ts">
  import { untrack } from 'svelte';
  import { api, type DeviceSummary } from '../lib/api';
  import { appConfig } from '../lib/config.svelte';
  import { deviceStatus, kindLabel } from '../lib/devices';
  import { formatAgo, formatBits, formatDuration, formatPercent } from '../lib/format';
  import { poll } from '../lib/poll';
  import { devicePath, hostPath } from '../lib/router.svelte';
  import Meter from '../components/Meter.svelte';
  import StatTile from '../components/StatTile.svelte';
  import StatusBadge from '../components/StatusBadge.svelte';

  // The SNMP devices agents poll: switches, UPSs, NAS boxes and the like

  let devices = $state<DeviceSummary[]>([]);
  let loaded = $state(false);
  let error = $state('');
  let now = $state(Math.floor(Date.now() / 1000));

  async function load() {
    now = Math.floor(Date.now() / 1000);
    try {
      devices = (await api.devices()).devices;
      appConfig.devices = devices.map((d) => d.name);
      error = '';
    } catch (e) {
      error = (e as Error).message;
    } finally {
      loaded = true;
    }
  }

  $effect(() => untrack(() => poll(load, appConfig.refreshSeconds)));

  // unreachable and stale devices first, then by name
  const rank = (d: DeviceSummary) => (d.error && !d.stale ? 0 : d.stale ? 1 : 2);
  const sorted = $derived([...devices].sort((a, b) => rank(a) - rank(b) || a.name.localeCompare(b.name)));
  const answering = $derived(devices.filter((d) => !d.error && !d.stale).length);
  const unreachable = $derived(devices.filter((d) => d.error && !d.stale).length);
  const stale = $derived(devices.filter((d) => d.stale).length);
</script>

<div class="page">
  <header class="page-header">
    <h1>Devices</h1>
  </header>

  {#if devices.length > 0}
    <section class="tiles" aria-label="Devices summary">
      <StatTile label="Devices" value={devices.length} />
      <StatTile label="Answering" value={answering} />
      <StatTile label="Unreachable" value={unreachable}>
        {#snippet extra()}
          {#if unreachable > 0}<StatusBadge status="critical" label="Not answering SNMP" />{/if}
        {/snippet}
      </StatTile>
      {#if stale > 0}
        <StatTile label="No new reports" value={stale}>
          {#snippet extra()}<StatusBadge status="warning" label="Agent not reporting" />{/snippet}
        </StatTile>
      {/if}
    </section>
  {/if}

  {#if error}
    <p class="card message">Could not load devices: {error}</p>
  {:else if loaded && devices.length === 0}
    <div class="card message">
      <h2>No devices report yet</h2>
      <p class="secondary">
        On a host on the same network as your switches, UPSs or NAS boxes, list them in <code>/etc/symon/snmp.json</code> with mode 600. Its
        agent polls them over SNMP v2c and reports here. The steps are in <code>docs/install.md</code> under Devices.
      </p>
    </div>
  {/if}

  <div class="grid">
    {#each sorted as device (device.name)}
      {@const badge = deviceStatus(device)}
      <a class="device card" href={devicePath(device.name)}>
        <div class="top">
          <span class="name">{device.name}</span>
          <StatusBadge status={badge.status} label={badge.label} />
        </div>
        <div class="meta muted">
          {kindLabel(device.kind)} · {device.address}
          {#if device.uptimeSeconds && !device.error}· up {formatDuration(device.uptimeSeconds)}{/if}
        </div>

        <!-- an unreachable device's numbers are old, its error is what matters -->
        {#if device.error && !device.stale}
          <p class="problem secondary">{device.error}</p>
        {:else}
          {#if device.upsOnBattery !== null || device.upsChargePct !== null}
            <div class="facts">
              {#if device.upsOnBattery}
                <StatusBadge status="warning" label="On battery" />
              {:else if device.upsOnBattery === false}
                <span class="secondary">On line power</span>
              {/if}
              {#if device.upsChargePct !== null}<span class="secondary num">Battery {formatPercent(device.upsChargePct)}</span>{/if}
              {#if device.upsMinutesLeft !== null}<span class="secondary num">{Math.round(device.upsMinutesLeft)} min left</span>{/if}
            </div>
          {/if}
          {#if device.cpuPct !== null || device.memUsedPct !== null || device.upsLoadPct !== null}
            <div class="meters">
              {#if device.upsLoadPct !== null}<Meter label="Load" value={device.upsLoadPct} />{/if}
              {#if device.cpuPct !== null}<Meter label="CPU" value={device.cpuPct} />{/if}
              {#if device.memUsedPct !== null}<Meter label="Memory" value={device.memUsedPct} />{/if}
            </div>
          {/if}
          {#if device.ports > 0}
            <div class="facts">
              <span class="secondary num">{device.portsUp} of {device.ports} ports up</span>
              <span class="secondary num">↓ {formatBits(device.rxBps)} · ↑ {formatBits(device.txBps)}</span>
            </div>
          {/if}
        {/if}

        <div class="polled muted">
          {#if device.host}polled by {device.host}{:else}its host was removed{/if}, {formatAgo(device.reportedAt, now)}
        </div>
      </a>
    {/each}
  </div>

  {#if devices.some((d) => d.host)}
    <p class="hosts muted">
      Polling hosts:
      {#each [...new Set(devices.map((d) => d.host).filter(Boolean))] as host, i (host)}{i > 0 ? ', ' : ''}<a href={hostPath(host)}>{host}</a>{/each}
    </p>
  {/if}
</div>

<style>
  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: 12px;
    margin-bottom: 16px;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
    gap: 12px;
  }

  .device {
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 14px;
    color: inherit;
    text-decoration: none;
    transition: border-color 0.15s;
  }

  .device:hover {
    border-color: var(--baseline);
    text-decoration: none;
  }

  .top {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
  }

  .name {
    font-weight: 600;
    font-size: 15px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .meta {
    font-size: 12px;
    margin-top: -6px;
  }

  .problem {
    margin: 0;
    font-size: 12px;
    overflow-wrap: anywhere;
  }

  .facts {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px 14px;
    font-size: 12px;
  }

  .meters {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .polled {
    margin-top: auto;
    font-size: 12px;
  }

  .hosts {
    margin: 16px 0 0;
    font-size: 12px;
  }

  .message {
    padding: 20px;
    margin: 0 0 16px;
  }

  .message p {
    margin: 6px 0 0;
  }

  code {
    font-size: 12px;
    padding: 1px 4px;
    border-radius: 4px;
    background: var(--hover);
  }
</style>
