<script lang="ts">
  import { formatPercent } from '../lib/format';

  // a 0 to 100 bar. The fill turns to warning and critical past the
  // thresholds, the value is always written next to it.
  let { label, value, warn = 80, critical = 90 }: { label: string; value: number; warn?: number; critical?: number } = $props();

  const level = $derived(value >= critical ? 'critical' : value >= warn ? 'warning' : 'normal');
</script>

<div class="meter">
  <span class="label secondary">{label}</span>
  <span class="track" role="meter" aria-label={label} aria-valuenow={Math.round(value)} aria-valuemin="0" aria-valuemax="100">
    <span class="fill {level}" style:width="{Math.max(0, Math.min(100, value))}%"></span>
  </span>
  <span class="value num">{formatPercent(value)}</span>
</div>

<style>
  .meter {
    display: grid;
    grid-template-columns: 64px 1fr 44px;
    align-items: center;
    gap: 8px;
    font-size: 12px;
  }

  .track {
    height: 6px;
    border-radius: 3px;
    background: var(--track);
    overflow: hidden;
  }

  .fill {
    display: block;
    height: 100%;
    border-radius: 3px;
    background: var(--accent);
  }

  .fill.warning {
    background: var(--warning);
  }

  .fill.critical {
    background: var(--critical);
  }

  .value {
    text-align: right;
  }
</style>
