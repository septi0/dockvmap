<script lang="ts">
  import type { Snippet } from "svelte";
  import { link } from "svelte-spa-router";

  let {
    label,
    value,
    tone = "neutral",
    valueStyle = "number",
    href,
    caption,
    captionTone = "muted",
    loading = false,
    title,
    icon,
  }: {
    label: string;
    value: string | number;
    tone?: "neutral" | "accent" | "danger" | "warning";
    valueStyle?: "number" | "badge";
    href?: string;
    caption?: string;
    captionTone?: "muted" | "danger" | "warning";
    loading?: boolean;
    title?: string;
    icon?: Snippet;
  } = $props();
</script>

{#snippet body()}
  <span class="tile-label">
    {#if icon}<span class="tile-icon">{@render icon()}</span>{/if}
    {label}
  </span>
  {#if loading}
    <span class="tile-value skeleton">&nbsp;</span>
  {:else if valueStyle === "badge"}
    <span class="tile-badge tone-{tone}">{value}</span>
  {:else}
    <span class="tile-value tone-{tone}">{value}</span>
  {/if}
  {#if caption && !loading}
    <span class="tile-caption caption-{captionTone}">{caption}</span>
  {/if}
{/snippet}

{#if href}
  <a class="stat-tile-card is-link" {href} {title} use:link>{@render body()}</a>
{:else}
  <div class="stat-tile-card" {title}>{@render body()}</div>
{/if}

<style>
  .stat-tile-card {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-4) var(--space-5);
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-sm);
    text-decoration: none;
    color: inherit;
  }

  .stat-tile-card.is-link {
    transition:
      border-color var(--transition-fast),
      background-color var(--transition-fast);
  }

  .stat-tile-card.is-link:hover {
    border-color: var(--color-border-strong);
    background: var(--color-surface-hover);
  }

  .tile-label {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    font-size: 0.75rem;
    font-weight: 500;
    letter-spacing: 0.02em;
    color: var(--color-text-muted);
  }

  .tile-icon {
    display: inline-flex;
    color: var(--color-text-faint);
  }

  .tile-value {
    font-size: 1.75rem;
    font-weight: 600;
    line-height: 1.15;
    letter-spacing: -0.02em;
  }

  .tile-value.tone-accent {
    color: var(--color-accent);
  }

  .tile-value.tone-danger {
    color: var(--color-danger);
  }

  .tile-value.tone-warning {
    color: var(--color-warning);
  }

  .tile-badge {
    align-self: flex-start;
    margin: var(--space-1) 0;
    padding: 3px var(--space-2);
    border-radius: var(--radius-full);
    font-size: 0.8125rem;
    font-weight: 600;
    line-height: 1.3;
    background: var(--color-surface-hover);
    color: var(--color-text-muted);
  }

  .tile-badge.tone-accent {
    background: var(--color-accent-muted-bg);
    color: var(--color-accent);
  }

  .tile-badge.tone-warning {
    background: var(--color-warning-bg);
    color: var(--color-warning);
  }

  .tile-badge.tone-danger {
    background: var(--color-danger-bg);
    color: var(--color-danger);
  }

  .tile-caption {
    font-size: 0.75rem;
  }

  .caption-muted {
    color: var(--color-text-muted);
  }

  .caption-danger {
    color: var(--color-danger);
  }

  .caption-warning {
    color: var(--color-warning);
  }

  .skeleton {
    width: 3ch;
    border-radius: var(--radius-sm);
    background: var(--color-surface-hover);
    animation: pulse 1.4s ease-in-out infinite;
  }

  @keyframes pulse {
    50% {
      opacity: 0.45;
    }
  }
</style>
