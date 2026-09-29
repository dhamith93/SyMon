<script lang="ts">
  import { ApiError } from '../lib/api';
  import { logIn } from '../lib/auth.svelte';

  let user = $state('');
  let password = $state('');
  let error = $state('');
  let busy = $state(false);

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    error = '';
    try {
      await logIn(user.trim(), password);
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) error = 'Wrong user name or password.';
      else if (e instanceof ApiError && e.status === 429) error = 'Too many failed logins for this user. Try again in 15 minutes.';
      else error = `Could not log in: ${(e as Error).message}`;
      password = '';
      busy = false;
    }
  }
</script>

<div class="page">
  <form class="card box" onsubmit={submit}>
    <h1>Log in</h1>
    <label>
      <span class="secondary">User name</span>
      <!-- svelte-ignore a11y_autofocus -->
      <input class="control" name="username" autocomplete="username" required autofocus bind:value={user} />
    </label>
    <label>
      <span class="secondary">Password</span>
      <input class="control" name="password" type="password" autocomplete="current-password" required bind:value={password} />
    </label>
    {#if error}<p class="error" role="alert">{error}</p>{/if}
    <button class="control submit" type="submit" disabled={busy}>{busy ? 'Logging in…' : 'Log in'}</button>
  </form>
</div>

<style>
  .box {
    display: flex;
    flex-direction: column;
    gap: 14px;
    max-width: 360px;
    margin: 48px auto 0;
    padding: 24px;
  }

  h1 {
    margin: 0 0 4px;
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

  .error {
    margin: 0;
    font-size: 13px;
    color: var(--critical-ink);
  }

  .submit {
    background: var(--accent);
    border-color: var(--accent);
    color: #fff;
    font-weight: 600;
  }

  .submit:hover {
    background: var(--accent-ink);
  }

  .submit:disabled {
    opacity: 0.7;
  }
</style>
