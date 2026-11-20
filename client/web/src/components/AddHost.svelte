<script lang="ts">
  import { api } from '../lib/api';

  let { primary = false }: { primary?: boolean } = $props();

  let dialog = $state<HTMLDialogElement>();
  let commandBox = $state<HTMLElement>();
  let host = $state('');
  let uses = $state(1);
  let hours = $state(1);
  let busy = $state(false);
  let error = $state('');
  let copied = $state('');
  let created = $state<{ command: string; expires: number; uses: number; host: string } | null>(null);

  function open() {
    reset();
    dialog?.showModal();
  }

  // the token goes when the dialog closes, so it is not left on the page
  function reset() {
    host = '';
    uses = 1;
    hours = 1;
    error = '';
    copied = '';
    created = null;
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    error = '';
    try {
      // a token for one host name enrolls that one host
      const name = host.trim();
      const count = name ? 1 : uses;
      const reply = await api.createEnrollToken(name, count, hours);
      created = { command: reply.command, expires: reply.expires, uses: count, host: name };
    } catch (e) {
      error = (e as Error).message.replace(/^invalid request: /, '');
    } finally {
      busy = false;
    }
  }

  // the clipboard only works over HTTPS or on localhost, so otherwise the
  // command is selected for the user to copy
  async function copy() {
    if (!created) return;
    try {
      await navigator.clipboard.writeText(created.command);
      copied = 'Copied.';
    } catch {
      if (commandBox) window.getSelection()?.selectAllChildren(commandBox);
      copied = 'Selected. Press Ctrl+C or ⌘C to copy.';
    }
  }

  function until(unixSeconds: number): string {
    return new Date(unixSeconds * 1000).toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
  }

  const reach = $derived.by(() => {
    if (!created) return '';
    const who = created.uses === 1 ? 'Works once' : `Works for ${created.uses} hosts`;
    const only = created.host ? `, only for a host named ${created.host}` : '';
    return `${who}${only}, until ${until(created.expires)}.`;
  });
</script>

<button class="control add" class:primary type="button" onclick={open}>Add host</button>

<dialog bind:this={dialog} onclose={reset} aria-labelledby="add-host-title">
  <div class="head">
    <h2 id="add-host-title">Add a host</h2>
    <button class="control close" type="button" onclick={() => dialog?.close()} aria-label="Close">×</button>
  </div>

  {#if created}
    <p class="secondary">
      Run this on the Linux host you want to monitor. It downloads the agent from this dashboard, enrolls it and starts it as the
      <code>symon_agent</code> service. The host appears here within a minute.
    </p>
    <pre bind:this={commandBox}><code>{created.command}</code></pre>
    <div class="row">
      <button class="control submit" type="button" onclick={copy}>Copy command</button>
      <button class="control" type="button" onclick={reset}>Make another</button>
      {#if copied}<span class="secondary" role="status">{copied}</span>{/if}
    </div>
    <p class="secondary small">
      {reach} The host needs to reach this dashboard and the collector, port 9000 unless it was changed.
    </p>
  {:else}
    <form onsubmit={submit}>
      <label>
        <span class="secondary">Host name, optional. Without one the host is named after the machine.</span>
        <input class="control" type="text" bind:value={host} maxlength="63" pattern="[A-Za-z0-9._\-]+" placeholder="web-3" autocomplete="off" />
      </label>
      <div class="pair">
        <label>
          <span class="secondary">Hosts it can add</span>
          <input class="control" type="number" bind:value={uses} min="1" max="100" required disabled={host.trim() !== ''} />
        </label>
        <label>
          <span class="secondary">Valid for</span>
          <select class="control" bind:value={hours}>
            <option value={1}>1 hour</option>
            <option value={24}>1 day</option>
            <option value={168}>7 days</option>
          </select>
        </label>
      </div>
      {#if error}<p class="error" role="alert">{error}</p>{/if}
      <button class="control submit" type="submit" disabled={busy}>{busy ? 'Creating…' : 'Create install command'}</button>
    </form>
  {/if}
</dialog>

<style>
  .add.primary,
  .submit {
    background: var(--accent);
    border-color: var(--accent);
    color: #fff;
    font-weight: 600;
  }

  .add.primary:hover,
  .submit:hover {
    background: var(--accent-ink);
  }

  dialog {
    width: min(560px, calc(100vw - 32px));
    padding: 20px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface-raised);
    color: var(--ink);
  }

  dialog::backdrop {
    background: rgba(0, 0, 0, 0.4);
  }

  .head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 12px;
  }

  h2 {
    font-size: 16px;
  }

  .close {
    width: 32px;
    padding: 0;
    font-size: 18px;
    line-height: 1;
  }

  form {
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  label {
    display: flex;
    flex-direction: column;
    gap: 6px;
    font-size: 13px;
  }

  .pair {
    display: flex;
    flex-wrap: wrap;
    gap: 14px;
  }

  .pair label {
    flex: 1 1 160px;
  }

  input,
  select {
    font-size: 14px;
  }

  .submit {
    align-self: flex-start;
  }

  p {
    margin: 0 0 12px;
  }

  .small {
    margin: 12px 0 0;
    font-size: 13px;
  }

  pre {
    margin: 0 0 12px;
    padding: 10px 12px;
    border-radius: var(--radius-sm);
    background: var(--hover);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    font-size: 12px;
    user-select: all;
  }

  .row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
  }

  .error {
    margin: 0;
    font-size: 13px;
    color: var(--critical-ink);
  }
</style>
