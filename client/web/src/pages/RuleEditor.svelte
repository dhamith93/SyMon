<script lang="ts">
  import { api, type Channel } from '../lib/api';
  import { isAdmin } from '../lib/auth.svelte';
  import { navigate } from '../lib/router.svelte';
  import {
    applyKindDefaults,
    deviceMetrics,
    formFromRule,
    isDevice,
    isKube,
    kinds,
    newForm,
    ops,
    ruleFromForm,
    thresholdKinds,
    type Kind,
    type RuleForm,
  } from '../lib/rules';

  // id 0 is a new rule
  let { id }: { id: number } = $props();

  let form = $state<RuleForm>(newForm());
  let fleetHosts = $state<string[]>([]);
  let clusters = $state<string[]>([]);
  let devices = $state<string[]>([]);
  let channels = $state<Channel[]>([]);
  let loaded = $state(false);
  let error = $state('');
  let busy = $state(false);
  let confirmDelete = $state(false);

  const editable = $derived(isAdmin());
  const isThreshold = $derived(thresholdKinds.includes(form.kind));
  const https = $derived(form.endpoint.trim().toLowerCase().startsWith('https://'));
  // hosts the rule names that are not registered any more still show
  const hostChoices = $derived([...new Set([...fleetHosts, ...form.hosts])].sort());
  // and so do channels it picks that are gone, so they can be unticked
  const missingChannels = $derived(form.channels.filter((name) => !channels.some((c) => c.name === name)));
  const defaultChannels = $derived(channels.filter((c) => c.isDefault && c.enabled).map((c) => c.name));
  const kube = $derived(isKube(form.kind));
  // a cluster the rule names that does not report any more still shows
  const clusterChoices = $derived([...new Set([...clusters, ...(form.cluster ? [form.cluster] : [])])].sort());
  const device = $derived(isDevice(form.kind));
  // and so does a device, "*" is every device
  const deviceChoices = $derived([...new Set([...devices, ...(form.device && form.device !== '*' ? [form.device] : [])])].sort());

  async function load() {
    try {
      fleetHosts = (await api.fleet()).hosts.map((h) => h.name);
      channels = (await api.channels()).channels;
      clusters = (await api.clusters().catch(() => ({ clusters: [] }))).clusters.map((c) => c.name);
      devices = (await api.devices().catch(() => ({ devices: [] }))).devices.map((d) => d.name);
      if (id) {
        const found = (await api.rules()).rules.find((r) => r.id === id);
        if (!found) {
          error = 'There is no such rule.';
          return;
        }
        form = formFromRule(found.rule, found.enabled, channels);
      }
    } catch (e) {
      error = (e as Error).message;
    } finally {
      loaded = true;
    }
  }

  function toggleChannel(name: string) {
    form.channels = form.channels.includes(name) ? form.channels.filter((c) => c !== name) : [...form.channels, name];
  }

  function toggleHost(host: string) {
    form.hosts = form.hosts.includes(host) ? form.hosts.filter((h) => h !== host) : [...form.hosts, host];
  }

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    error = '';
    try {
      const rule = ruleFromForm(form);
      if (id) await api.updateRule(id, form.enabled, rule);
      else await api.createRule(form.enabled, rule);
      navigate('/rules');
    } catch (e) {
      error = (e as Error).message.replace(/^invalid request: /, '');
      busy = false;
    }
  }

  async function remove() {
    busy = true;
    try {
      await api.deleteRule(id);
      navigate('/rules');
    } catch (e) {
      error = (e as Error).message;
      busy = false;
    }
  }

  load();
</script>

<div class="page">
  <header class="page-header">
    <p class="crumbs muted"><a href="/rules">Alert rules</a> /</p>
    <h1>{id ? form.name || 'Rule' : 'New rule'}</h1>
    {#if !editable}<p class="muted">Only admins can change rules.</p>{/if}
  </header>

  {#if !loaded}
    <p class="muted">Loading…</p>
  {:else}
    <form class="card box" onsubmit={save}>
      <fieldset disabled={!editable || busy}>
        <div class="row">
          <label class="grow">
            <span>Name</span>
            <input class="control" required maxlength="100" bind:value={form.name} placeholder="Data disk filling up" />
          </label>
          <label class="check">
            <input type="checkbox" bind:checked={form.enabled} />
            <span>On</span>
          </label>
        </div>

        <label>
          <span>Watch</span>
          <select class="control" value={form.kind} onchange={(e) => applyKindDefaults(form, e.currentTarget.value as Kind)}>
            {#each kinds as kind (kind.value)}<option value={kind.value}>{kind.label}</option>{/each}
          </select>
        </label>

        {#if form.kind === 'custom'}
          <label>
            <span>Custom metric name</span>
            <input class="control" required bind:value={form.customName} placeholder="queue-length" />
          </label>
        {/if}
        {#if form.kind === 'disks' || form.kind === 'disk_forecast'}
          <label>
            <span>Disk device, as in the host's disk table</span>
            <input class="control" required bind:value={form.disk} placeholder="/dev/sda1" />
          </label>
        {/if}
        {#if form.kind === 'services'}
          <div class="row">
            <label class="grow">
              <span>Service, from the agent's service list</span>
              <input class="control" required bind:value={form.service} placeholder="nginx" />
            </label>
            <label>
              <span>Alert when it</span>
              <select class="control" bind:value={form.op}>
                <option value="inactive">stops</option>
                <option value="active">runs</option>
              </select>
            </label>
          </div>
        {/if}

        {#if form.kind === 'endpoint'}
          <div class="row">
            <label class="narrow">
              <span>Method</span>
              <select class="control" bind:value={form.method}>
                <option>GET</option>
                <option>HEAD</option>
                <option>POST</option>
              </select>
            </label>
            <label class="grow">
              <span>URL</span>
              <input class="control" type="url" required bind:value={form.endpoint} placeholder="https://shop.example.com/health" />
            </label>
            <label class="narrow">
              <span>Expected status</span>
              <input class="control" type="number" min="100" max="599" bind:value={form.expected} />
            </label>
          </div>
          {#if form.method === 'POST'}
            <div class="row">
              <label class="grow">
                <span>Body</span>
                <input class="control" bind:value={form.postBody} placeholder={'{}'} />
              </label>
              <label>
                <span>Content type</span>
                <input class="control" bind:value={form.postContentType} />
              </label>
            </div>
          {/if}
          <label>
            <span>CA certificate file on the collector's host, for a private CA</span>
            <input class="control" bind:value={form.customCACert} placeholder="/etc/symon/ca.pem" />
          </label>
          {#if https}
            <div class="row">
              <label>
                <span>Certificate warning under, days</span>
                <input class="control" type="number" min="0" step="1" bind:value={form.certWarn} placeholder="14" />
              </label>
              <label>
                <span>Critical under, days</span>
                <input class="control" type="number" min="0" step="1" bind:value={form.certCritical} placeholder="3" />
              </label>
            </div>
            <p class="hint muted">Empty uses 14 and 3 days. 0 switches a level off.</p>
          {/if}
        {:else if kube}
          <div class="row">
            <label class="grow">
              <span>Cluster</span>
              {#if clusterChoices.length > 0}
                <select class="control" required bind:value={form.cluster}>
                  <option value="" disabled>Pick a cluster</option>
                  {#each clusterChoices as name (name)}<option>{name}</option>{/each}
                </select>
              {:else}
                <input class="control" required bind:value={form.cluster} placeholder="homelab" />
              {/if}
            </label>
            {#if form.kind !== 'kube_node'}
              <label class="grow">
                <span>Namespace, empty for all</span>
                <input class="control" bind:value={form.namespace} placeholder="media" />
              </label>
            {/if}
          </div>
          {#if clusterChoices.length === 0}
            <p class="hint muted">No cluster reports yet. The name is the agent's SYMON_KUBE_CLUSTER.</p>
          {/if}
        {:else if device}
          <div class="row">
            <label class="grow">
              <span>Device</span>
              <select class="control" required bind:value={form.device}>
                <option value="*">Every device, including ones added later</option>
                {#each deviceChoices as name (name)}<option>{name}</option>{/each}
              </select>
            </label>
            {#if form.kind === 'device_port'}
              <label class="grow">
                <span>Port, as on the device's page</span>
                <input class="control" required bind:value={form.iface} placeholder="Gi1/0/24" />
              </label>
            {/if}
          </div>
          {#if form.kind === 'device_metric'}
            <div class="row">
              <label class="grow">
                <span>Value</span>
                <input class="control" required list="device-metrics" bind:value={form.deviceMetric} placeholder="ups_charge_pct" />
                <datalist id="device-metrics">
                  {#each deviceMetrics as metric (metric)}<option value={metric}></option>{/each}
                </datalist>
              </label>
              <label class="grow">
                <span>Port, volume or line, empty for every one</span>
                <input class="control" bind:value={form.label} placeholder="/volume1" />
              </label>
            </div>
            <p class="hint muted">A metric from the devices file works too, by its name. Port traffic is in bits per second.</p>
          {:else if form.kind === 'device_port'}
            <p class="hint muted">Only a port that is switched on and has no link alerts. A port switched off on the device does not.</p>
          {/if}
          {#if deviceChoices.length === 0}
            <p class="hint muted">No device reports yet. The names come from the devices file on the polling host.</p>
          {/if}
        {:else}
          <div class="hosts">
            <span>Hosts</span>
            <label class="check">
              <input type="radio" name="hosts" checked={form.allHosts} onchange={() => (form.allHosts = true)} />
              <span>All hosts, including ones added later</span>
            </label>
            <label class="check">
              <input type="radio" name="hosts" checked={!form.allHosts} onchange={() => (form.allHosts = false)} />
              <span>These hosts</span>
            </label>
            {#if !form.allHosts}
              <div class="choices">
                {#each hostChoices as host (host)}
                  <label class="chip" class:selected={form.hosts.includes(host)}>
                    <input type="checkbox" checked={form.hosts.includes(host)} onchange={() => toggleHost(host)} />
                    {host}
                  </label>
                {/each}
              </div>
            {/if}
          </div>
        {/if}

        {#if isThreshold}
          <div class="row">
            <label class="narrow">
              <span>When the value is</span>
              <select class="control" bind:value={form.op}>
                {#each ops as op (op)}<option>{op}</option>{/each}
              </select>
            </label>
            <label>
              <span>Warning at</span>
              <input class="control" type="number" step="1" required bind:value={form.warn} />
            </label>
            <label>
              <span>Critical at</span>
              <input class="control" type="number" step="1" required bind:value={form.critical} />
            </label>
          </div>
        {/if}

        <label>
          <span>{form.kind === 'ping' ? 'Alert when a host sends nothing for, seconds' : 'Alert once it lasts, seconds'}</span>
          {#if form.kind === 'kube_restarts'}<span class="hint muted">Counts the restarts of each workload's pods in the last hour.</span>{/if}
          <input class="control" type="number" min="0" step="1" bind:value={form.trigger} />
        </label>

        <div class="notify">
          <span>Send alerts to</span>
          {#if channels.length === 0 && missingChannels.length === 0}
            <p class="muted">
              There are no notification channels yet, so this rule's alerts only show on the dashboard.
              {#if editable}<a href="/notifications">Add a channel</a>{/if}
            </p>
          {:else}
            <div class="row">
              {#each channels as channel (channel.id)}
                <label class="check">
                  <input type="checkbox" checked={form.channels.includes(channel.name)} onchange={() => toggleChannel(channel.name)} />
                  <span>{channel.name}{channel.enabled ? '' : ' (off)'}</span>
                </label>
              {/each}
              {#each missingChannels as name (name)}
                <label class="check">
                  <input type="checkbox" checked onchange={() => toggleChannel(name)} />
                  <span>{name} (deleted)</span>
                </label>
              {/each}
            </div>
            {#if form.channels.length === 0}
              <p class="muted">
                {defaultChannels.length > 0
                  ? `None picked, so alerts go to the default ${defaultChannels.length === 1 ? 'channel' : 'channels'}: ${defaultChannels.join(', ')}.`
                  : 'None picked and no channel is a default, so alerts only show on the dashboard.'}
              </p>
            {/if}
          {/if}
        </div>

        <details>
          <summary class="secondary">Message and description</summary>
          <label>
            <span>Message template</span>
            <textarea class="control" rows="3" bind:value={form.template}></textarea>
          </label>
          <label>
            <span>Description, for {'{desc}'} in the template</span>
            <input class="control" bind:value={form.description} />
          </label>
        </details>
      </fieldset>

      {#if error}<p class="error" role="alert">{error}</p>{/if}

      {#if editable}
        <div class="actions">
          <button class="control primary" type="submit" disabled={busy}>{id ? 'Save' : 'Create rule'}</button>
          <a class="control cancel" href="/rules">Cancel</a>
          {#if id}
            <span class="spacer"></span>
            {#if confirmDelete}
              <span class="secondary">Delete it? Its open alerts resolve.</span>
              <button class="control danger" type="button" disabled={busy} onclick={remove}>Delete</button>
              <button class="control" type="button" onclick={() => (confirmDelete = false)}>Keep</button>
            {:else}
              <button class="control" type="button" onclick={() => (confirmDelete = true)}>Delete rule</button>
            {/if}
          {/if}
        </div>
      {/if}
    </form>
  {/if}
</div>

<style>
  .crumbs {
    margin: 0 0 2px;
    font-size: 12px;
  }

  .box {
    max-width: 760px;
    padding: 20px;
  }

  fieldset {
    display: flex;
    flex-direction: column;
    gap: 14px;
    margin: 0;
    padding: 0;
    border: none;
    min-width: 0;
  }

  label,
  .hosts,
  .notify {
    display: flex;
    flex-direction: column;
    gap: 6px;
    font-size: 13px;
    color: var(--ink-secondary);
  }

  .notify p {
    margin: 0;
  }

  .row {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    gap: 12px;
  }

  .grow {
    flex: 1 1 260px;
  }

  .narrow {
    flex: 0 0 auto;
  }

  label.check {
    flex-direction: row;
    align-items: center;
    gap: 6px;
    color: var(--ink);
  }

  input.control,
  select.control,
  textarea.control {
    font-size: 14px;
    color: var(--ink);
  }

  textarea.control {
    height: auto;
    padding: 6px 10px;
    font-family: ui-monospace, monospace;
    font-size: 12px;
  }

  .choices {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }

  .chip {
    flex-direction: row;
    align-items: center;
    gap: 6px;
    padding: 4px 10px;
    border: 1px solid var(--border);
    border-radius: 999px;
    color: var(--ink);
    cursor: pointer;
  }

  .chip.selected {
    border-color: var(--accent);
    background: var(--hover);
  }

  .chip input {
    margin: 0;
  }

  .hint {
    margin: -8px 0 0;
    font-size: 12px;
  }

  details {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  details[open] summary {
    margin-bottom: 10px;
  }

  summary {
    cursor: pointer;
    font-size: 13px;
  }

  .error {
    margin: 14px 0 0;
    font-size: 13px;
    color: var(--critical-ink);
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
    margin-top: 18px;
  }

  .spacer {
    flex: 1;
  }

  .primary {
    background: var(--accent);
    border-color: var(--accent);
    color: #fff;
    font-weight: 600;
  }

  .primary:hover {
    background: var(--accent-ink);
  }

  .cancel {
    display: inline-flex;
    align-items: center;
    color: var(--ink);
    text-decoration: none;
  }

  .danger {
    background: var(--critical);
    border-color: var(--critical);
    color: #fff;
  }
</style>
