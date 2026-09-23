<script lang="ts">
  import type { Snippet } from 'svelte';

  // a label and one number. extra holds a status badge or a hint.
  let { label, value, extra }: { label: string; value: string | number; extra?: Snippet } = $props();
</script>

<div class="tile card">
  <span class="label secondary">{label}</span>
  <span class="value">{value}</span>
  {#if extra}<span class="extra">{@render extra()}</span>{/if}
</div>

<style>
  .tile {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 12px 14px;
    min-width: 0;
  }

  .label {
    font-size: 12px;
  }

  .value {
    font-size: 26px;
    font-weight: 600;
    line-height: 1.2;
  }

  .extra {
    font-size: 12px;
  }
</style>
