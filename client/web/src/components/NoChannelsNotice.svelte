<script lang="ts">
  import { api } from '../lib/api';
  import { isAdmin } from '../lib/auth.svelte';

  // Says so when no notification channel is on, since alerts then reach
  // nobody who is not looking at the dashboard
  let show = $state(false);

  api
    .channels()
    .then((response) => (show = !response.channels.some((c) => c.enabled)))
    .catch(() => (show = false));
</script>

{#if show}
  <p class="notice card" role="status">
    <span>No notification channel is on, so alerts only show here.</span>
    {#if isAdmin()}<a href="/notifications">Add a channel</a>{:else}<span class="muted">An admin can add one.</span>{/if}
  </p>
{/if}

<style>
  .notice {
    display: flex;
    flex-wrap: wrap;
    gap: 6px 12px;
    align-items: center;
    margin: 0 0 12px;
    padding: 10px 14px;
    font-size: 13px;
    border-left: 3px solid var(--warning);
  }
</style>
