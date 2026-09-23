<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import type { NamedSeries } from '../lib/align';
  import { formatDateTime, formatPercent, formatTime } from '../lib/format';
  import { cssVar, theme } from '../lib/theme.svelte';

  // one row per series, colored by value from 0 to 100. from and to set
  // the time span, so columns line up with the line charts beside it.
  let { series, from, to }: { series: NamedSeries[]; from?: number; to?: number } = $props();

  let wrapper: HTMLDivElement;
  let canvas: HTMLCanvasElement;
  let hover = $state<{ left: number; top: number; label: string; time: number; value: number } | null>(null);
  let resizeObserver: ResizeObserver | null = null;

  const labelWidth = 44;
  const axisHeight = 20;

  const grid = $derived.by(() => {
    const timeSet = new Set<number>();
    for (const s of series) for (const [time] of s.points) timeSet.add(time);
    const times = [...timeSet].sort((a, b) => a - b);
    const index = new Map(times.map((time, i) => [time, i]));
    const rows = series.map((s) => {
      const row: (number | null)[] = new Array(times.length).fill(null);
      for (const [time, value] of s.points) row[index.get(time)!] = value;
      return row;
    });
    // a column is as wide as the typical gap between samples
    const deltas = times.slice(1).map((time, i) => time - times[i]).sort((a, b) => a - b);
    const step = deltas[Math.floor(deltas.length / 2)] ?? 60;
    const start = from ?? times[0] ?? 0;
    const end = to ?? (times[times.length - 1] ?? 0) + step;
    return { times, labels: series.map((s) => s.label), rows, step, start, end };
  });

  const rowHeight = $derived(Math.max(6, Math.min(18, Math.floor(160 / Math.max(1, series.length)))));
  const height = $derived(rowHeight * series.length + axisHeight);

  function hexToRgb(hex: string): [number, number, number] {
    const value = parseInt(hex.replace('#', ''), 16);
    return [(value >> 16) & 255, (value >> 8) & 255, value & 255];
  }

  function ramp(): [number, number, number][] {
    return Array.from({ length: 7 }, (_, i) => hexToRgb(cssVar(`--heat-${i}`)));
  }

  function colorFor(value: number, steps: [number, number, number][]): string {
    const t = Math.max(0, Math.min(1, value / 100)) * (steps.length - 1);
    const i = Math.min(steps.length - 2, Math.floor(t));
    const f = t - i;
    const mix = steps[i].map((c, k) => Math.round(c + (steps[i + 1][k] - c) * f));
    return `rgb(${mix[0]}, ${mix[1]}, ${mix[2]})`;
  }

  function draw() {
    if (!canvas || !wrapper) return;
    const width = wrapper.clientWidth;
    const ratio = window.devicePixelRatio || 1;
    canvas.width = width * ratio;
    canvas.height = height * ratio;
    canvas.style.width = `${width}px`;
    canvas.style.height = `${height}px`;
    const ctx = canvas.getContext('2d')!;
    ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
    ctx.clearRect(0, 0, width, height);

    const { times, labels, rows, step, start, end } = grid;
    if (times.length === 0 || end <= start) return;
    const steps = ramp();
    const plotWidth = width - labelWidth;
    const xOf = (time: number) => labelWidth + ((time - start) / (end - start)) * plotWidth;
    const cellWidth = Math.max(1, Math.ceil((step / (end - start)) * plotWidth));

    rows.forEach((row, r) => {
      row.forEach((value, c) => {
        if (value == null || times[c] < start || times[c] >= end) return;
        ctx.fillStyle = colorFor(value, steps);
        // a hairline of surface between rows keeps them apart
        ctx.fillRect(xOf(times[c]), r * rowHeight, cellWidth, rowHeight - 1);
      });
    });

    ctx.fillStyle = cssVar('--ink-muted');
    ctx.font = '11px system-ui, -apple-system, "Segoe UI", sans-serif';
    ctx.textBaseline = 'middle';
    const labelEvery = Math.ceil(12 / rowHeight);
    labels.forEach((label, r) => {
      if (r % labelEvery === 0) ctx.fillText(`cpu ${label}`, 0, r * rowHeight + rowHeight / 2);
    });

    const span = end - start;
    ctx.textBaseline = 'alphabetic';
    const y = rows.length * rowHeight + 15;
    ctx.textAlign = 'left';
    ctx.fillText(formatTime(start, span), labelWidth, y);
    ctx.textAlign = 'right';
    ctx.fillText(formatTime(end, span), width, y);
    ctx.textAlign = 'left';
  }

  function onMove(event: MouseEvent) {
    const rect = canvas.getBoundingClientRect();
    const x = event.clientX - rect.left - labelWidth;
    const y = event.clientY - rect.top;
    const { times, labels, rows, step, start, end } = grid;
    const time = start + (x / (rect.width - labelWidth)) * (end - start);
    // the column under the pointer is the last sample at or before it
    let c = -1;
    for (let i = 0; i < times.length && times[i] <= time; i++) c = i;
    const r = Math.floor(y / rowHeight);
    const value = rows[r]?.[c];
    if (x < 0 || c < 0 || time - times[c] > step || value == null) {
      hover = null;
      return;
    }
    hover = { left: event.clientX - rect.left, top: y, label: labels[r], time: times[c], value };
  }

  $effect(() => {
    // redraw when the data, size or theme change
    void grid;
    void height;
    void theme.resolved;
    draw();
  });

  onMount(() => {
    resizeObserver = new ResizeObserver(draw);
    resizeObserver.observe(wrapper);
  });

  onDestroy(() => resizeObserver?.disconnect());
</script>

<div class="heatmap" bind:this={wrapper}>
  <canvas bind:this={canvas} onmousemove={onMove} onmouseleave={() => (hover = null)} aria-label="CPU usage per core over time"></canvas>
  {#if hover}
    <div class="tooltip" style:left="{hover.left + 14}px" style:top="{hover.top}px">
      <div class="muted">{formatDateTime(hover.time)}</div>
      <strong class="num">{formatPercent(hover.value)}</strong> <span class="secondary">cpu {hover.label}</span>
    </div>
  {/if}
  <div class="scale">
    <span>0%</span>
    <span class="bar"></span>
    <span>100%</span>
  </div>
</div>

<style>
  .heatmap {
    position: relative;
    padding-top: 6px;
  }

  canvas {
    display: block;
  }

  .tooltip {
    position: absolute;
    z-index: 5;
    pointer-events: none;
    padding: 6px 10px;
    background: var(--surface-raised);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 4px 14px rgba(0, 0, 0, 0.12);
    font-size: 12px;
    white-space: nowrap;
  }

  .scale {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 6px 0 4px 44px;
    font-size: 11px;
    color: var(--ink-muted);
  }

  .bar {
    width: 120px;
    height: 8px;
    border-radius: 4px;
    background: linear-gradient(
      to right,
      var(--heat-0),
      var(--heat-1),
      var(--heat-2),
      var(--heat-3),
      var(--heat-4),
      var(--heat-5),
      var(--heat-6)
    );
  }
</style>
