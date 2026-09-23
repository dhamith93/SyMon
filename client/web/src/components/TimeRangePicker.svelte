<script lang="ts">
  import { navigate, location } from '../lib/router.svelte';
  import { fromLocalInput, presets, rangeQuery, toLocalInput, type TimeRange } from '../lib/timerange';

  // presets first, a custom window behind them. The choice lives in the URL.
  let { range }: { range: TimeRange } = $props();

  let customOpen = $state(false);
  let customFrom = $state('');
  let customTo = $state('');

  function pick(preset: string) {
    customOpen = false;
    navigate(location.path + rangeQuery(location.query, { preset }), true);
  }

  function openCustom() {
    customFrom = toLocalInput(range.from);
    customTo = toLocalInput(range.to);
    customOpen = !customOpen;
  }

  function applyCustom(event: SubmitEvent) {
    event.preventDefault();
    const from = fromLocalInput(customFrom);
    const to = fromLocalInput(customTo);
    if (from && to && to > from) {
      customOpen = false;
      navigate(location.path + rangeQuery(location.query, { from, to }), true);
    }
  }
</script>

<div class="picker">
  <div class="presets" role="group" aria-label="Time range">
    {#each presets as preset (preset.key)}
      <button class:selected={range.preset === preset.key} aria-pressed={range.preset === preset.key} onclick={() => pick(preset.key)}>
        {preset.label}
      </button>
    {/each}
    <button class:selected={range.preset === ''} aria-pressed={range.preset === ''} aria-expanded={customOpen} onclick={openCustom}>
      Custom
    </button>
  </div>
  {#if customOpen}
    <form class="custom card" onsubmit={applyCustom}>
      <label>From <input class="control" type="datetime-local" bind:value={customFrom} required /></label>
      <label>To <input class="control" type="datetime-local" bind:value={customTo} required /></label>
      <button class="control apply" type="submit">Apply</button>
    </form>
  {/if}
</div>

<style>
  .picker {
    position: relative;
  }

  .presets {
    display: inline-flex;
    flex-wrap: wrap;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--surface-raised);
    overflow: hidden;
  }

  .presets button {
    border: none;
    border-right: 1px solid var(--border);
    background: none;
    padding: 6px 12px;
    color: var(--ink-secondary);
  }

  .presets button:last-child {
    border-right: none;
  }

  .presets button:hover {
    background: var(--hover);
  }

  .presets button.selected {
    color: var(--ink);
    font-weight: 600;
    background: var(--hover);
    box-shadow: inset 0 -2px 0 var(--accent);
  }

  .custom {
    position: absolute;
    z-index: 10;
    top: calc(100% + 6px);
    left: 0;
    display: flex;
    flex-wrap: wrap;
    align-items: end;
    gap: 10px;
    padding: 12px;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.12);
  }

  .custom label {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 12px;
    color: var(--ink-secondary);
  }

  .apply {
    background: var(--accent);
    border-color: var(--accent);
    color: #fff;
  }

  .apply:hover {
    background: var(--accent-ink);
  }
</style>
