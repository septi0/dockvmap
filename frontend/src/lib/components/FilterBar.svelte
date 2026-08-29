<script lang="ts">
  import type { Snippet } from "svelte";
  import FilterX from "@lucide/svelte/icons/filter-x";

  let {
    active = false,
    onClear,
    children,
  }: {
    active?: boolean;
    onClear?: () => void;
    children: Snippet;
  } = $props();
</script>

<div class="filter-bar">
  {@render children()}

  {#if active && onClear}
    <button type="button" class="clear" onclick={onClear}>
      <FilterX size={14} strokeWidth={1.75} />
      Clear filters
    </button>
  {/if}
</div>

<style>
  .filter-bar {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-2) var(--space-3);
    min-height: 32px;
    margin-bottom: var(--space-4);
  }

  .filter-bar :global(.filter-field) {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
  }

  .filter-bar :global(.filter-label) {
    font-size: 0.8125rem;
    font-weight: 500;
    color: var(--color-text-muted);
    white-space: nowrap;
    transition: color var(--transition-fast);
  }

  .filter-bar :global(.filter-field:has(.is-active) .filter-label) {
    color: var(--color-text);
  }

  .clear {
    margin-left: auto;
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    padding: var(--space-1) var(--space-2);
    border: none;
    border-radius: var(--radius-sm);
    background: none;
    font: inherit;
    font-size: 0.8125rem;
    font-weight: 500;
    line-height: 1;
    color: var(--color-text-muted);
    cursor: pointer;
    transition:
      color var(--transition-fast),
      background-color var(--transition-fast);
  }

  .clear:hover {
    color: var(--color-text);
    background: var(--color-surface-hover);
  }

  .clear:focus-visible {
    outline: 2px solid var(--color-accent);
    outline-offset: 2px;
  }
</style>
