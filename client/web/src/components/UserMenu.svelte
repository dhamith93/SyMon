<script lang="ts">
  import { auth, logOut } from '../lib/auth.svelte';
  import { location } from '../lib/router.svelte';
  import { setTheme, theme, type ThemeChoice } from '../lib/theme.svelte';

  const themes: { value: ThemeChoice; label: string }[] = [
    { value: 'system', label: 'System' },
    { value: 'light', label: 'Light' },
    { value: 'dark', label: 'Dark' },
  ];

  let open = $state(false);
  let root: HTMLElement;
  let trigger: HTMLButtonElement;
  const signedIn = $derived(auth.state === 'in');

  // going to another page, the account link included, closes it
  $effect(() => {
    void location.path;
    open = false;
  });

  function outside(e: MouseEvent) {
    if (open && !root.contains(e.target as Node)) open = false;
  }

  function leave() {
    open = false;
    logOut();
  }

  function escape(e: KeyboardEvent) {
    if (open && e.key === 'Escape') {
      open = false;
      trigger.focus();
    }
  }
</script>

<svelte:document onclick={outside} onkeydown={escape} />

<div class="menu" bind:this={root}>
  <button bind:this={trigger} class="trigger" aria-expanded={open} aria-controls="user-menu" onclick={() => (open = !open)}>
    {#if signedIn}
      <span class="avatar" aria-hidden="true">{auth.user.slice(0, 1).toUpperCase()}</span>
      <span class="name">{auth.user}</span>
    {:else}
      Theme
    {/if}
    <svg viewBox="0 0 12 12" width="10" height="10" aria-hidden="true"><path d="M2.5 4.5 6 8l3.5-3.5" fill="none" stroke="currentColor" stroke-width="1.5" /></svg>
  </button>

  <div id="user-menu" class="panel" hidden={!open}>
    {#if signedIn}
      <div class="who">
        <strong class="mono">{auth.user}</strong>
        <span class="muted">{auth.role === 'admin' ? 'Admin' : 'Viewer'}</span>
      </div>
      <a class="item" href="/account">Account and password</a>
      <div class="sep"></div>
    {/if}
    <fieldset class="themes">
      <legend>Theme</legend>
      <div class="segments">
        {#each themes as t (t.value)}
          <label class:selected={theme.choice === t.value}>
            <input type="radio" name="theme" value={t.value} checked={theme.choice === t.value} onchange={() => setTheme(t.value)} />
            {t.label}
          </label>
        {/each}
      </div>
    </fieldset>
    {#if signedIn}
      <div class="sep"></div>
      <button class="item" onclick={leave}>Log out</button>
    {/if}
  </div>
</div>

<style>
  .menu {
    position: relative;
    margin-left: auto;
  }

  .trigger {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 4px 6px;
    border: 1px solid transparent;
    border-radius: var(--radius-sm);
    background: none;
    color: var(--ink-secondary);
    font-size: 13px;
  }

  .trigger:hover,
  .trigger[aria-expanded='true'] {
    border-color: var(--border);
    color: var(--ink);
  }

  .avatar {
    display: grid;
    place-items: center;
    width: 22px;
    height: 22px;
    border-radius: var(--radius-sm);
    background: var(--track);
    color: var(--accent-ink);
    font: 600 12px/1 var(--font-mono);
  }

  .name {
    max-width: 16ch;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-mono);
  }

  .panel {
    position: absolute;
    right: 0;
    top: calc(100% + 6px);
    z-index: 40;
    width: 220px;
    padding: 6px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface-raised);
    box-shadow: 0 6px 20px rgba(0, 0, 0, 0.12);
  }

  .who {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 8px;
    padding: 6px 8px 8px;
    border-bottom: 1px solid var(--rule);
    margin-bottom: 4px;
    font-size: 13px;
  }

  .who strong {
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .item {
    display: block;
    width: 100%;
    padding: 7px 8px;
    border: 0;
    border-radius: var(--radius-sm);
    background: none;
    color: var(--ink);
    font-size: 13px;
    text-align: left;
    text-decoration: none;
  }

  .item:hover {
    background: var(--hover);
  }

  .sep {
    margin: 4px 0;
    border-top: 1px solid var(--rule);
  }

  .themes {
    margin: 0;
    padding: 6px 8px 8px;
    border: 0;
  }

  .themes legend {
    float: left;
    width: 100%;
    margin-bottom: 6px;
    color: var(--ink-muted);
    font-size: 12px;
  }

  .segments {
    clear: both;
    display: flex;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
  }

  .segments label {
    position: relative;
    flex: 1;
    padding: 4px 0;
    color: var(--ink-secondary);
    font-size: 12px;
    text-align: center;
    cursor: pointer;
  }

  .segments label + label {
    border-left: 1px solid var(--border);
  }

  .segments label.selected {
    background: var(--hover);
    color: var(--ink);
    font-weight: 500;
  }

  .segments label:has(:focus-visible) {
    outline: 2px solid var(--accent);
    outline-offset: -2px;
  }

  /* covers its label, so the whole segment is the radio */
  .segments input {
    position: absolute;
    inset: 0;
    margin: 0;
    opacity: 0;
    cursor: pointer;
  }
</style>
