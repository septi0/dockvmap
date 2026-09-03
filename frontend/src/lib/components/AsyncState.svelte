<script lang="ts">
  import type { Snippet } from "svelte";
  import LoaderCircle from "@lucide/svelte/icons/loader-circle";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Inbox from "@lucide/svelte/icons/inbox";
  import TableSkeleton from "./TableSkeleton.svelte";
  import ListSkeleton from "./ListSkeleton.svelte";
  import { resolveAsyncState } from "../utils/asyncState";

  let {
    loading,
    error,
    empty = false,
    busy = false,
    loadingMessage = "Loading…",
    emptyMessage = "Nothing here yet.",
    emptyVariant = "card",
    columns,
    rows,
    listSkeleton = false,
    loadingState,
    emptyAction,
    children,
  }: {
    loading: boolean;
    error: string | null;
    empty?: boolean;
    busy?: boolean;
    loadingMessage?: string;
    emptyMessage?: string;
    emptyVariant?: "card" | "text";
    columns?: number;
    rows?: number;
    listSkeleton?: boolean;
    loadingState?: Snippet;
    emptyAction?: Snippet;
    children: Snippet;
  } = $props();

  let state = $derived(resolveAsyncState({ loading, hasError: !!error, empty, busy }));
</script>

{#if state.kind === "loading"}
  {#if loadingState}
    {@render loadingState()}
  {:else if columns}
    <TableSkeleton {columns} {rows} />
  {:else if listSkeleton}
    <ListSkeleton {rows} />
  {:else}
    <div class="loading">
      <span class="spinner spin"><LoaderCircle size={20} strokeWidth={3} /></span>
      <span class="muted">{loadingMessage}</span>
    </div>
  {/if}
{:else if state.kind === "error"}
  <p class="error">
    <TriangleAlert size={16} strokeWidth={2} />
    {error}
  </p>
{:else if state.kind === "empty"}
  {#if emptyVariant === "text"}
    <p class="empty-text muted">{emptyMessage}</p>
  {:else}
    <div class="empty card">
      <span class="empty-icon"><Inbox size={28} strokeWidth={1.5} /></span>
      <p>{emptyMessage}</p>
      {#if emptyAction}
        <div class="empty-action">{@render emptyAction()}</div>
      {/if}
    </div>
  {/if}
{:else}
  <div class="result" class:busy-dim={state.busy}>
    {@render children()}
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
    flex-shrink: 0;
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

  .empty-text {
    margin: 0;
    padding: var(--space-2) 0;
  }

  .result {
    transition: opacity var(--transition-fast);
  }
</style>
