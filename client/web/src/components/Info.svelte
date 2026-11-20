<script lang="ts">
  // an (i) that shows a note on hover, focus or tap, for text people only need now and then
  interface Props {
    text: string;
    // what the button is called for screen readers
    label?: string;
  }

  let { text, label = 'More about this' }: Props = $props();

  const id = $props.id();
  let open = $state(false);
  let button: HTMLButtonElement;
  let tip: HTMLElement;
  let top = $state(0);
  let left = $state(0);

  // fixed, so it never widens the page, kept inside the window and closed on scroll
  $effect(() => {
    if (!open) return;
    const rect = button.getBoundingClientRect();
    top = rect.bottom + 6;
    left = Math.max(8, Math.min(rect.left - 8, window.innerWidth - tip.offsetWidth - 8));

    const close = () => (open = false);
    const outside = (e: PointerEvent) => {
      if (!button.contains(e.target as Node)) open = false;
    };
    window.addEventListener('scroll', close, { capture: true, passive: true });
    window.addEventListener('resize', close);
    document.addEventListener('pointerdown', outside);
    return () => {
      window.removeEventListener('scroll', close, { capture: true });
      window.removeEventListener('resize', close);
      document.removeEventListener('pointerdown', outside);
    };
  });
</script>

<button
  bind:this={button}
  type="button"
  class="info"
  aria-label={label}
  aria-describedby={id}
  onmouseenter={() => (open = true)}
  onmouseleave={() => (open = false)}
  onfocus={() => (open = true)}
  onblur={() => (open = false)}
  onclick={() => (open = true)}
  onkeydown={(e) => e.key === 'Escape' && (open = false)}
>
  <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
    <circle cx="8" cy="8" r="6.5" fill="none" stroke="currentColor" stroke-width="1.3" />
    <path d="M8 7.2v4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
    <circle cx="8" cy="4.9" r="0.95" fill="currentColor" />
  </svg>
</button>
<span bind:this={tip} {id} role="tooltip" class="tip" class:open style:top="{top}px" style:left="{left}px">{text}</span>

<style>
  .info {
    display: inline-flex;
    vertical-align: middle;
    padding: 3px;
    border: 0;
    border-radius: var(--radius-sm);
    background: none;
    color: var(--ink-muted);
    cursor: help;
  }

  .info:hover,
  .info:focus-visible {
    color: var(--ink);
  }

  .tip {
    display: none;
    position: fixed;
    z-index: 30;
    width: max-content;
    max-width: min(300px, calc(100vw - 16px));
    padding: 7px 10px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--surface-raised);
    box-shadow: 0 4px 14px rgba(0, 0, 0, 0.12);
    color: var(--ink-secondary);
    font: 400 12.5px/1.45 var(--font-sans);
    letter-spacing: normal;
    text-transform: none;
    white-space: normal;
    text-align: left;
  }

  .tip.open {
    display: block;
  }
</style>
