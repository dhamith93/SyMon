<script lang="ts">
  import uPlot from 'uplot';
  import 'uplot/dist/uPlot.min.css';
  import { onDestroy, onMount } from 'svelte';
  import type { Aligned } from '../lib/align';
  import { formatAxis, formatDateTime, formatValue, type Unit } from '../lib/format';
  import { cssVar, theme } from '../lib/theme.svelte';

  interface Props {
    data: Aligned;
    unit: Unit;
    // fixed top of the y axis, like 100 for percentages
    yMax?: number;
    // charts with the same key share a cursor
    syncKey: string;
    // the x axis spans the chosen range even where there is no data
    from?: number;
    to?: number;
    // soft fill under a single series
    area?: boolean;
    height?: number;
    onzoom?: (from: number, to: number) => void;
    onpick?: (time: number) => void;
  }

  let { data, unit, yMax, syncKey, from, to, area = false, height = 180, onzoom, onpick }: Props = $props();

  let wrapper: HTMLDivElement;
  // uPlot owns this element, Svelte owns the tooltip next to it
  let target: HTMLDivElement;
  let plot: uPlot | null = null;
  let builtFor = '';
  let resizeObserver: ResizeObserver | null = null;
  // synced charts all move their cursor, only the one under the pointer shows a tooltip
  let pointerInside = false;

  interface TooltipRow {
    label: string;
    color: string;
    value: string;
  }
  let tooltip = $state<{ left: number; top: number; time: number; rows: TooltipRow[]; flipped: boolean } | null>(null);

  const font = '12px system-ui, -apple-system, "Segoe UI", sans-serif';

  function seriesColors(): string[] {
    return Array.from({ length: 8 }, (_, i) => cssVar(`--series-${i + 1}`));
  }

  function plotData(d: Aligned): uPlot.AlignedData {
    return [d.times, ...d.values] as uPlot.AlignedData;
  }

  function build() {
    plot?.destroy();
    const colors = seriesColors();
    const muted = cssVar('--ink-muted');
    const grid = cssVar('--grid');
    const single = data.labels.length === 1;

    const options: uPlot.Options = {
      width: wrapper.clientWidth,
      height,
      legend: { show: false },
      padding: [8, 12, 0, 0],
      cursor: {
        sync: { key: syncKey },
        drag: { x: true, y: false, setScale: false },
        points: { size: 8, width: 2, stroke: cssVar('--surface') },
      },
      scales: {
        x: { time: true, range: (_u, min, max) => [from ?? min, to ?? max] },
        y: { range: (_u, _min, max) => [0, yMax ?? (max > 0 ? max * 1.1 : 1)] },
      },
      axes: [
        { stroke: muted, font, grid: { stroke: grid, width: 1 }, ticks: { show: false }, space: 70 },
        {
          stroke: muted,
          font,
          size: 72,
          grid: { stroke: grid, width: 1 },
          ticks: { show: false },
          values: (_u, splits) => formatAxis(splits, unit),
        },
      ],
      series: [
        {},
        ...data.labels.map((label, i) => ({
          label,
          stroke: colors[i],
          width: 2,
          fill: single && area ? colors[i] + '1a' : undefined,
          points: { show: false },
        })),
      ],
      hooks: {
        setCursor: [(u) => updateTooltip(u, colors)],
        setSelect: [
          (u) => {
            if (u.select.width < 5) return;
            const from = u.posToVal(u.select.left, 'x');
            const to = u.posToVal(u.select.left + u.select.width, 'x');
            u.setSelect({ left: 0, top: 0, width: 0, height: 0 }, false);
            onzoom?.(Math.floor(from), Math.ceil(to));
          },
        ],
      },
    };
    plot = new uPlot(options, plotData(data), target);
    attachPick(plot);
  }

  // a click without a drag picks a moment, like "processes at this time"
  function attachPick(u: uPlot) {
    let downX = 0;
    u.over.addEventListener('mousedown', (e) => (downX = e.clientX));
    u.over.addEventListener('click', (e) => {
      if (!onpick || Math.abs(e.clientX - downX) > 4 || u.cursor.idx == null) return;
      const time = u.data[0][u.cursor.idx];
      if (Number.isInteger(time)) onpick(time);
    });
    u.over.style.cursor = onpick ? 'crosshair' : 'default';
  }

  function updateTooltip(u: uPlot, colors: string[]) {
    const idx = u.cursor.idx;
    if (!pointerInside || idx == null || u.cursor.left == null || u.cursor.left < 0) {
      tooltip = null;
      return;
    }
    const rows = data.labels
      .map((label, i) => ({ label, color: colors[i], raw: u.data[i + 1][idx] }))
      .filter((row) => row.raw != null)
      .map((row) => ({ label: row.label, color: row.color, value: formatValue(row.raw as number, unit) }));
    if (rows.length === 0) {
      tooltip = null;
      return;
    }
    const left = u.over.offsetLeft + u.cursor.left;
    const flip = left > wrapper.clientWidth - 200;
    tooltip = {
      left: flip ? left - 16 : left + 16,
      top: u.over.offsetTop + Math.max(0, (u.cursor.top ?? 0) - 20),
      time: u.data[0][idx],
      rows,
      flipped: flip,
    };
  }

  // rebuild when the series or the theme change, otherwise swap the data in
  $effect(() => {
    const key = [theme.resolved, unit, yMax, height, area, ...data.labels].join('|');
    // a new range only needs the x scale to move, not a rebuild
    void from;
    void to;
    if (!target) return;
    if (!plot || key !== builtFor) {
      builtFor = key;
      build();
    } else {
      plot.setData(plotData(data));
      plot.setScale('x', { min: from ?? data.times[0], max: to ?? data.times[data.times.length - 1] });
    }
  });

  onMount(() => {
    resizeObserver = new ResizeObserver(() => plot?.setSize({ width: wrapper.clientWidth, height }));
    resizeObserver.observe(wrapper);
  });

  onDestroy(() => {
    resizeObserver?.disconnect();
    plot?.destroy();
  });
</script>

<div
  class="chart"
  bind:this={wrapper}
  onmouseenter={() => (pointerInside = true)}
  onmouseleave={() => {
    pointerInside = false;
    tooltip = null;
  }}
  role="presentation"
>
  <div bind:this={target}></div>
  {#if tooltip}
    <div class="tooltip" class:flipped={tooltip.flipped} style:left="{tooltip.left}px" style:top="{tooltip.top}px">
      <div class="time">{formatDateTime(Math.round(tooltip.time))}</div>
      {#each tooltip.rows as row (row.label)}
        <div class="row">
          <span class="key" style:background={row.color}></span>
          <strong class="num">{row.value}</strong>
          <span class="label">{row.label}</span>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .chart {
    position: relative;
    width: 100%;
  }

  .chart :global(.u-select) {
    background: var(--hover);
  }

  .chart :global(.u-cursor-x) {
    border-right: 1px solid var(--baseline);
  }

  .chart :global(.u-cursor-y) {
    display: none;
  }

  .tooltip {
    position: absolute;
    z-index: 5;
    pointer-events: none;
    min-width: 140px;
    max-width: 260px;
    padding: 8px 10px;
    background: var(--surface-raised);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 4px 14px rgba(0, 0, 0, 0.12);
    font-size: 12px;
  }

  .tooltip.flipped {
    transform: translateX(-100%);
  }

  .time {
    color: var(--ink-muted);
    margin-bottom: 4px;
  }

  .row {
    display: flex;
    align-items: center;
    gap: 6px;
    line-height: 1.6;
  }

  .key {
    width: 12px;
    height: 2px;
    border-radius: 1px;
    flex: none;
  }

  .label {
    color: var(--ink-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
