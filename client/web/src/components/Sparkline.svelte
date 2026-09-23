<script lang="ts">
  // a small line of recent values, 0 to 100
  let { points, label }: { points: [number, number][]; label: string } = $props();

  const width = 120;
  const height = 28;

  const path = $derived.by(() => {
    if (points.length < 2) return '';
    const first = points[0][0];
    const span = points[points.length - 1][0] - first || 1;
    return points
      .map(([time, value], i) => {
        const x = ((time - first) / span) * width;
        const y = height - 1 - (Math.max(0, Math.min(100, value)) / 100) * (height - 2);
        return `${i === 0 ? 'M' : 'L'}${x.toFixed(1)},${y.toFixed(1)}`;
      })
      .join(' ');
  });
</script>

{#if path}
  <svg viewBox="0 0 {width} {height}" width={width} height={height} role="img" aria-label={label}>
    <path d={path} fill="none" stroke="var(--series-1)" stroke-width="1.5" stroke-linejoin="round" stroke-linecap="round" />
  </svg>
{/if}
