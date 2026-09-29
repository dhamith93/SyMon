<script lang="ts">
  import { api } from '../lib/api';
  import { isAdmin } from '../lib/auth.svelte';
  import { navigate } from '../lib/router.svelte';
  import { applyKindDefaults, formFromRule, kinds, newForm, ops, ruleFromForm, thresholdKinds, type Kind, type RuleForm } from '../lib/rules';

  // id 0 is a new rule
  let { id }: { id: number } = $props();

  let form = $state<RuleForm>(newForm());
  let fleetHosts = $state<string[]>([]);
  let loaded = $state(false);
  let error = $state('');
  let busy = $state(false);
  let confirmDelete = $state(false);

  const editable = $derived(isAdmin());
  const isThreshold = $derived(thresholdKinds.includes(form.kind));
  const https = $derived(form.endpoint.trim().toLowerCase().startsWith('https://'));
  // hosts the rule names that are not registered any more still show
  const hostChoices = $derived([...new Set([...fleetHosts, ...form.hosts])].sort());

  async function load() {
    try {
      fleetHosts = (await api.fleet()).hosts.map((h) => h.name);
      if (id) {
        const found = (await api.rules()).rules.find((r) => r.id === id);
        if (!found) {
          error = 'There is no such rule.';
          return;
        }
        form = formFromRule(found.rule, found.enabled);
      }
    } catch (e) {
      error = (e as Error).message;
    } finally {
      loaded = true;
    }
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
          <input class="control" type="number" min="0" step="1" bind:value={form.trigger} />
        </label>

        <div class="notify">
          <span>Also send to, through the alert processor</span>
          <div class="row">
            <label class="check"><input type="checkbox" bind:checked={form.email} /><span>Email</span></label>
            <label class="check"><input type="checkbox" bind:checked={form.slack} /><span>Slack</span></label>
            <label class="check"><input type="checkbox" bind:checked={form.pagerduty} /><span>PagerDuty</span></label>
          </div>
          {#if form.slack}
            <label>
              <span>Slack channel</span>
              <input class="control" bind:value={form.slackChannel} placeholder="#ops" />
            </label>
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
