<script lang="ts">
  import type { Snippet } from "svelte";
  import LoaderCircle from "@lucide/svelte/icons/loader-circle";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Inbox from "@lucide/svelte/icons/inbox";
  import TableSkeleton from "./TableSkeleton.svelte";

  let {
    loading,
    error,
    empty = false,
    busy = false,
    loadingMessage = "Loading…",
    emptyMessage = "Nothing here yet.",
    columns,
    rows,
    emptyAction,
    children,
  }: {
    loading: boolean;
    error: string | null;
    empty?: boolean;
    busy?: boolean;
    loadingMessage?: string;
    emptyMessage?: string;
    columns?: number;
    rows?: number;
    emptyAction?: Snippet;
    children: Snippet;
  } = $props();
</script>

{#if loading}
  {#if columns}
    <TableSkeleton {columns} {rows} />
  {:else}
    <div class="loading">
      <span class="spinner"><LoaderCircle size={20} strokeWidth={3} /></span>
      <span class="muted">{loadingMessage}</span>
    </div>
  {/if}
{:else if error}
  <p class="error">
    <TriangleAlert size={16} strokeWidth={2} />
    {error}
  </p>
{:else if empty}
  <div class="empty card">
    <span class="empty-icon"><Inbox size={28} strokeWidth={1.5} /></span>
    <p>{emptyMessage}</p>
    {#if emptyAction}
      <div class="empty-action">{@render emptyAction()}</div>
    {/if}
  </div>
{:else}
  <div class="result-wrap">
    {#if busy}
      <span class="result-spinner" aria-hidden="true">
        <LoaderCircle size={18} strokeWidth={3} />
      </span>
    {/if}
    <div class="result" class:result-busy={busy}>
      {@render children()}
    </div>
  </div>
{/if}

<style>
  .loading {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-4) 0;
  }

  .spinner {
    display: inline-flex;
    color: var(--color-accent);
    animation: spin 0.8s linear infinite;
    flex-shrink: 0;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-6);
    text-align: center;
  }

  :global(.card) .empty {
    border: none;
    box-shadow: none;
  }

  .empty-icon {
    display: inline-flex;
    color: var(--color-text-faint);
  }

  .empty p {
    margin: 0;
    color: var(--color-text-muted);
  }

  .empty-action {
    margin-top: var(--space-2);
  }

  .result-wrap {
    position: relative;
  }

  .result {
    transition: opacity var(--transition-fast);
  }

  .result-busy {
    opacity: 0.55;
    pointer-events: none;
  }

  .result-spinner {
    position: absolute;
    top: var(--space-3);
    right: var(--space-3);
    z-index: 2;
    display: inline-flex;
    color: var(--color-accent);
    animation: spin 0.8s linear infinite;
  }
</style>
