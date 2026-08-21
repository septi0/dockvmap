<script lang="ts">
  import type { Snippet } from "svelte";

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
    <button type="button" class="clear" onclick={onClear}>Clear filters</button>
  {/if}
</div>

<style>
  .filter-bar {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-3);
    margin-bottom: var(--space-4);
  }

  .clear {
    background: none;
    border: none;
    padding: 0;
    font: inherit;
    font-size: 0.8125rem;
    color: var(--color-accent);
    cursor: pointer;
    text-decoration: underline;
  }
</style>
