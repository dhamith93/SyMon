<script lang="ts">
  // status always comes with an icon and a word, never color alone
  type Status = 'up' | 'down' | 'warning' | 'critical' | 'resolved';
  let { status, label }: { status: Status; label?: string } = $props();

  const text: Record<Status, string> = {
    up: 'Up',
    down: 'Down',
    warning: 'Warning',
    critical: 'Critical',
    resolved: 'Resolved',
  };
</script>

<span class="badge {status}">
  <svg viewBox="0 0 12 12" width="12" height="12" aria-hidden="true">
    {#if status === 'up' || status === 'resolved'}
      <circle cx="6" cy="6" r="5" fill="var(--good)" />
      <path d="M3.5 6.2l1.7 1.7 3.3-3.6" fill="none" stroke="#fff" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
    {:else if status === 'warning'}
      <path d="M6 1l5 9.5H1z" fill="var(--warning)" />
      <path d="M6 4.5v3M6 9v.1" stroke="#0b0b0b" stroke-width="1.3" stroke-linecap="round" />
    {:else}
      <circle cx="6" cy="6" r="5" fill="var(--critical)" />
      <path d="M4 4l4 4M8 4l-4 4" stroke="#fff" stroke-width="1.5" stroke-linecap="round" />
    {/if}
  </svg>
  {label ?? text[status]}
</span>

<style>
  .badge {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    font-size: 12px;
    font-weight: 500;
    color: var(--ink-secondary);
    white-space: nowrap;
  }
</style>
