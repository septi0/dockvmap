<script lang="ts">
  import ArrowDown from "@lucide/svelte/icons/arrow-down";
  import ArrowUp from "@lucide/svelte/icons/arrow-up";
  import ArrowUpDown from "@lucide/svelte/icons/arrow-up-down";
  import type { SortDirection } from "../api/types/sort";

  let {
    label,
    direction,
    onSort,
    class: className = "",
  }: {
    label: string;
    direction: SortDirection | null;
    onSort: () => void;
    class?: string;
  } = $props();

  const ARIA_SORT = { asc: "ascending", desc: "descending" } as const;

  let ariaSort: "ascending" | "descending" | "none" = $derived(
    direction ? ARIA_SORT[direction] : "none",
  );
</script>

<th class={className} aria-sort={ariaSort}>
  <button type="button" class="sort-button" class:active={!!direction} onclick={onSort}>
    {label}
    <span class="sort-icon">
      {#if direction === "asc"}
        <ArrowUp size={12} strokeWidth={2} />
      {:else if direction === "desc"}
        <ArrowDown size={12} strokeWidth={2} />
      {:else}
        <ArrowUpDown size={12} strokeWidth={2} />
      {/if}
    </span>
  </button>
</th>

<style>
  .sort-button {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    padding: 0;
    border: none;
    background: none;
    font: inherit;
    color: inherit;
    white-space: nowrap;
    cursor: pointer;
  }

  .sort-icon {
    display: inline-flex;
    color: var(--color-text-faint);
    opacity: 0.6;
  }

  .sort-button:hover,
  .sort-button.active {
    color: var(--color-text);
  }

  .sort-button:hover .sort-icon,
  .sort-button.active .sort-icon {
    color: inherit;
    opacity: 1;
  }
</style>
