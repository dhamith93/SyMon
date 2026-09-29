<script lang="ts">
  import { api, ApiError } from '../lib/api';
  import { auth } from '../lib/auth.svelte';

  let current = $state('');
  let next = $state('');
  let repeat = $state('');
  let error = $state('');
  let done = $state(false);
  let busy = $state(false);

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    error = '';
    done = false;
    if (next !== repeat) {
      error = 'The new passwords do not match.';
      return;
    }
    busy = true;
    try {
      await api.changePassword(current, next);
      done = true;
      current = next = repeat = '';
    } catch (e) {
      if (e instanceof ApiError && e.status === 429) error = 'Too many wrong passwords. Try again in 15 minutes.';
      else error = (e as Error).message.replace(/^invalid request: /, '');
    } finally {
      busy = false;
    }
  }
</script>

<div class="page">
  <header class="page-header">
    <h1>{auth.user}</h1>
    <p class="secondary">
      {auth.role === 'admin' ? 'Admin: you can change alert rules.' : 'Viewer: you can see everything but not change alert rules.'}
    </p>
  </header>

  <form class="card box" onsubmit={submit}>
    <h2>Change your password</h2>
    <label>
      <span class="secondary">Current password</span>
      <input class="control" type="password" autocomplete="current-password" required bind:value={current} />
    </label>
    <label>
      <span class="secondary">New password, at least 12 characters</span>
      <input class="control" type="password" autocomplete="new-password" minlength="12" required bind:value={next} />
    </label>
    <label>
      <span class="secondary">New password again</span>
      <input class="control" type="password" autocomplete="new-password" minlength="12" required bind:value={repeat} />
    </label>
    {#if error}<p class="error" role="alert">{error}</p>{/if}
    {#if done}<p class="done" role="status">Password changed. You are logged out everywhere else.</p>{/if}
    <button class="control submit" type="submit" disabled={busy}>{busy ? 'Changing…' : 'Change password'}</button>
  </form>
</div>

<style>
  .page-header p {
    margin: 4px 0 0;
  }

  .box {
    display: flex;
    flex-direction: column;
    gap: 14px;
    max-width: 400px;
    padding: 20px;
  }

  h2 {
    font-size: 15px;
    margin: 0;
  }

  label {
    display: flex;
    flex-direction: column;
    gap: 6px;
    font-size: 13px;
  }

  input {
    font-size: 14px;
  }

  .error,
  .done {
    margin: 0;
    font-size: 13px;
  }

  .error {
    color: var(--critical-ink);
  }

  .done {
    color: var(--good-ink);
  }

  .submit {
    align-self: flex-start;
    background: var(--accent);
    border-color: var(--accent);
    color: #fff;
    font-weight: 600;
  }

  .submit:hover {
    background: var(--accent-ink);
  }
</style>
