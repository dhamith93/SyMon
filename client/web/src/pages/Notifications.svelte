<script lang="ts">
  import { untrack } from 'svelte';
  import { api, type Channel, type ChannelKind, type Notification } from '../lib/api';
  import { isAdmin } from '../lib/auth.svelte';
  import { formatAgo, formatDateTime } from '../lib/format';
  import {
    deliveryState,
    describeChannel,
    describeReach,
    eventName,
    formFromChannel,
    inputFromForm,
    kindFields,
    kindNames,
    kinds,
    newChannelForm,
    type ChannelForm,
  } from '../lib/notifications';
  import { poll } from '../lib/poll';
  import StatusBadge from '../components/StatusBadge.svelte';

  // Where alerts are sent. Admins set channels up here, and everyone can
  // see whether notifications go out.

  let channels = $state<Channel[]>([]);
  let recent = $state<Notification[]>([]);
  let loaded = $state(false);
  let error = $state('');
  let now = $state(Date.now() / 1000);

  // the channel being edited: 0 for a new one, null for none
  let editing = $state<number | null>(null);
  let form = $state<ChannelForm>(newChannelForm());
  let formError = $state('');
  let busy = $state(false);
  let confirmDelete = $state(0);
  // the latest Send test answer per channel
  let tests = $state<Record<number, { ok: boolean; error: string } | 'sending'>>({});

  async function load() {
    try {
      const [channelList, notificationList] = await Promise.all([api.channels(), api.notifications(50)]);
      channels = channelList.channels;
      recent = notificationList.notifications;
      error = '';
    } catch (e) {
      error = (e as Error).message;
    } finally {
      now = Date.now() / 1000;
      loaded = true;
    }
  }

  $effect(() => untrack(() => poll(load, 10)));

  function startNew() {
    form = newChannelForm();
    formError = '';
    editing = 0;
  }

  function startEdit(channel: Channel) {
    form = formFromChannel(channel);
    formError = '';
    editing = channel.id;
  }

  function pickKind(kind: ChannelKind) {
    form = { ...newChannelForm(kind), name: form.name, isDefault: form.isDefault, skipWarnings: form.skipWarnings };
  }

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    formError = '';
    try {
      const input = inputFromForm(form);
      if (editing) await api.updateChannel(editing, input);
      else await api.createChannel(input);
      editing = null;
      await load();
    } catch (e) {
      formError = (e as Error).message;
    } finally {
      busy = false;
    }
  }

  async function remove(id: number) {
    busy = true;
    try {
      await api.deleteChannel(id);
      confirmDelete = 0;
      if (editing === id) editing = null;
      await load();
    } catch (e) {
      error = (e as Error).message;
    } finally {
      busy = false;
    }
  }

  async function test(id: number) {
    tests[id] = 'sending';
    try {
      tests[id] = await api.testChannel(id);
    } catch (e) {
      tests[id] = { ok: false, error: (e as Error).message };
    }
  }

  const fields = $derived(kindFields[form.kind]);
  const namePlaceholders: Record<ChannelKind, string> = { email: 'ops email', slack: 'team slack', pagerduty: 'on call', webhook: 'home assistant' };
</script>

{#snippet state(status: string, lastError?: string)}
  {@const shown = deliveryState(status, lastError)}
  {#if shown.tone === 'busy'}
    <span class="busy">{shown.label}</span>
  {:else}
    <StatusBadge status={shown.tone === 'good' ? 'up' : shown.tone === 'bad' ? 'critical' : 'warning'} label={shown.label} />
  {/if}
{/snippet}

{#snippet channelForm()}
  <form class="card box" onsubmit={save}>
    <h2>{editing ? `Edit ${form.name}` : 'New channel'}</h2>
    <div class="fields">
      {#if !editing}
        <fieldset class="kinds">
          <legend>Kind</legend>
          {#each kinds as kind (kind)}
            <label class="check">
              <input type="radio" name="kind" checked={form.kind === kind} onchange={() => pickKind(kind)} />
              <span>{kindNames[kind]}</span>
            </label>
          {/each}
        </fieldset>
      {/if}
      <label>
        <span>Name</span>
        <input class="control" bind:value={form.name} disabled={!!editing} required maxlength="64" placeholder={namePlaceholders[form.kind]} />
        {#if !editing}<small class="muted">Rules pick channels by name, so it cannot change later.</small>{/if}
      </label>
      {#each fields as field (field.key)}
        <label>
          <span>{field.label}{field.required ? '' : ' (optional)'}</span>
          <input
            class="control"
            type={field.type === 'password' ? 'password' : field.type === 'number' ? 'number' : 'text'}
            autocomplete={field.secret ? 'new-password' : 'off'}
            bind:value={form.values[field.key]}
            placeholder={field.secret && form.saved.includes(field.key) ? 'saved, leave empty to keep' : field.placeholder}
            required={field.required && !(field.secret && form.saved.includes(field.key))}
          />
          {#if field.hint}<small class="muted">{field.hint}</small>{/if}
        </label>
      {/each}
      <label class="check"><input type="checkbox" bind:checked={form.isDefault} /><span>Default: also gets alerts from rules that pick no channel</span></label>
      <label class="check"><input type="checkbox" bind:checked={form.skipWarnings} /><span>Only critical alerts, skip warnings</span></label>
      <label class="check"><input type="checkbox" bind:checked={form.enabled} /><span>On</span></label>
    </div>
    {#if formError}<p class="error" role="alert">{formError}</p>{/if}
    <div class="actions">
      <button class="control primary" type="submit" disabled={busy}>{editing ? 'Save' : 'Add channel'}</button>
      <button class="control" type="button" onclick={() => (editing = null)}>Cancel</button>
    </div>
  </form>
{/snippet}

<div class="page">
  <header class="page-header top">
    <div>
      <p class="crumbs muted"><a href="/rules">Alert rules</a> /</p>
      <h1>Notifications</h1>
      <p class="secondary">
        Where alerts go when they open, change and resolve. Rules pick channels, and rules that pick none go to the default channels.
        {#if !isAdmin()}Only admins can change them.{/if}
      </p>
    </div>
    {#if isAdmin() && editing === null}<button class="control new" onclick={startNew}>Add channel</button>{/if}
  </header>

  {#if error}<p class="error" role="alert">{error}</p>{/if}

  {#if editing === 0}{@render channelForm()}{/if}

  <section>
    <h2>Channels</h2>
    <div class="card list">
      {#if !loaded}
        <p class="none muted">Loading…</p>
      {:else if channels.length === 0}
        <p class="none secondary">No channels yet, so alerts only show on the dashboard.</p>
      {:else}
        <div class="table-scroll">
          <table class="data">
            <caption class="sr-only">Notification channels</caption>
            <thead>
              <tr>
                <th>Channel</th>
                <th>Gets</th>
                <th>Newest notification</th>
                {#if isAdmin()}<th class="right"></th>{/if}
              </tr>
            </thead>
            <tbody>
              {#each channels as channel (channel.id)}
                {@const result = tests[channel.id]}
                <tr class:off={!channel.enabled}>
                  <td class="wrap">
                    <strong>{channel.name}</strong>
                    <div class="secondary">{kindNames[channel.kind]}{isAdmin() ? `, to ${describeChannel(channel)}` : ''}</div>
                  </td>
                  <td class="wrap secondary">{describeReach(channel)}</td>
                  <td class="wrap">
                    {#if channel.lastStatus}
                      {@render state(channel.lastStatus, channel.lastError)}
                      <span class="muted when" title={formatDateTime(channel.lastAt)}>{formatAgo(channel.lastAt, now)}</span>
                      {#if channel.lastError}<div class="secondary small">{channel.lastError}</div>{/if}
                    {:else}
                      <span class="muted">None yet</span>
                    {/if}
                  </td>
                  {#if isAdmin()}
                    <td class="right buttons">
                      <button class="control small" disabled={result === 'sending'} onclick={() => test(channel.id)}>
                        {result === 'sending' ? 'Sending…' : 'Send test'}
                      </button>
                      <button class="control small" onclick={() => startEdit(channel)}>Edit</button>
                      {#if confirmDelete === channel.id}
                        <button class="control small danger" disabled={busy} onclick={() => remove(channel.id)}>Delete</button>
                        <button class="control small" onclick={() => (confirmDelete = 0)}>Keep</button>
                      {:else}
                        <button class="control small" onclick={() => (confirmDelete = channel.id)}>Delete</button>
                      {/if}
                      {#if result && result !== 'sending'}
                        <div class="test" role="status">
                          {#if result.ok}
                            <StatusBadge status="up" label="Test sent" />
                          {:else}
                            <StatusBadge status="critical" label="Test failed" /> <span class="secondary">{result.error}</span>
                          {/if}
                        </div>
                      {/if}
                    </td>
                  {/if}
                </tr>
                {#if editing === channel.id}
                  <tr class="editing">
                    <td colspan="4">{@render channelForm()}</td>
                  </tr>
                {/if}
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  </section>

  <section>
    <h2>Recent notifications</h2>
    <div class="card list">
      {#if loaded && recent.length === 0}
        <p class="none secondary">Nothing sent yet.</p>
      {:else if recent.length > 0}
        <div class="table-scroll">
          <table class="data">
            <caption class="sr-only">Recent notifications, newest first</caption>
            <thead>
              <tr>
                <th>When</th>
                <th>Alert</th>
                <th>Event</th>
                <th>Channel</th>
                <th>Status</th>
                <th class="right">Tries</th>
              </tr>
            </thead>
            <tbody>
              {#each recent as n (n.id)}
                <tr>
                  <td class="num" title={formatAgo(n.createdAt, now)}>{formatDateTime(n.createdAt)}</td>
                  <td class="wrap">{n.subject}</td>
                  <td class="secondary">{eventName(n)}</td>
                  <td class="secondary">{n.channel}</td>
                  <td class="wrap">
                    {@render state(n.status, n.lastError)}
                    {#if n.lastError}<div class="secondary small">{n.lastError}</div>{/if}
                  </td>
                  <td class="right num">{n.attempts}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  </section>
</div>

<style>
  .crumbs {
    margin: 0 0 2px;
    font-size: 12px;
  }

  .top {
    display: flex;
    justify-content: space-between;
    align-items: flex-end;
    gap: 16px;
    flex-wrap: wrap;
  }

  .top p {
    margin: 4px 0 0;
  }

  section {
    margin-bottom: 24px;
  }

  h2 {
    font-size: 15px;
    margin: 0 0 10px;
  }

  .list {
    padding: 6px 4px;
  }

  .none {
    margin: 0;
    padding: 24px;
    text-align: center;
  }

  .wrap {
    white-space: normal;
    min-width: 160px;
  }

  .small {
    font-size: 12px;
  }

  .when {
    margin-left: 6px;
    font-size: 12px;
  }

  tr.off td {
    opacity: 0.6;
  }

  td.buttons {
    white-space: normal;
  }

  td.buttons .control {
    margin: 2px 0 2px 4px;
  }

  .test {
    margin-top: 6px;
    font-size: 12px;
  }

  tr.editing:hover {
    background: none;
  }

  tr.editing td {
    padding: 8px 4px 14px;
    white-space: normal;
  }

  .busy {
    font-size: 12px;
    font-weight: 500;
    color: var(--ink-secondary);
  }

  .box {
    max-width: 640px;
    padding: 18px 20px;
    margin-bottom: 16px;
  }

  .fields {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  label {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 13px;
    color: var(--ink-secondary);
  }

  label.check {
    flex-direction: row;
    align-items: center;
    gap: 6px;
    color: var(--ink);
  }

  .kinds {
    display: flex;
    flex-wrap: wrap;
    gap: 6px 16px;
    margin: 0;
    padding: 0;
    border: none;
  }

  .kinds legend {
    padding: 0;
    margin-bottom: 6px;
    font-size: 13px;
    color: var(--ink-secondary);
  }

  input.control {
    width: 100%;
    max-width: 420px;
    box-sizing: border-box;
  }

  .actions {
    display: flex;
    gap: 10px;
    margin-top: 16px;
  }

  .error {
    margin: 0 0 12px;
    font-size: 13px;
    color: var(--critical-ink);
  }

  .box .error {
    margin: 12px 0 0;
  }

  .small.control {
    height: 26px;
    font-size: 12px;
  }

  .control:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .primary,
  .new {
    background: var(--accent);
    border-color: var(--accent);
    color: #fff;
    font-weight: 600;
  }

  .primary:hover,
  .new:hover {
    background: var(--accent-ink);
  }

  .danger {
    background: var(--critical);
    border-color: var(--critical);
    color: #fff;
  }
</style>
