<script lang="ts">
  import type { Snippet } from "svelte";
  import LoaderCircle from "@lucide/svelte/icons/loader-circle";
  import { resolveAsyncState } from "../../utils/asyncState";

  let {
    loading = false,
    busy = false,
    hasError = false,
    empty = false,
    errorState,
    emptyState,
    children,
  }: {
    loading?: boolean;
    busy?: boolean;
    hasError?: boolean;
    empty?: boolean;
    errorState?: Snippet;
    emptyState?: Snippet;
    children: Snippet;
  } = $props();

  let state = $derived(resolveAsyncState({ loading, hasError, empty, busy }));
</script>

<div class="card-body">
  {#if state.kind === "loading"}
    <div class="card-body-fill">
      <span class="card-body-spinner spin" aria-hidden="true">
        <LoaderCircle size={20} strokeWidth={3} />
      </span>
    </div>
  {:else if state.kind === "error" && errorState}
    {@render errorState()}
  {:else if state.kind === "empty" && emptyState}
    {@render emptyState()}
  {:else}
    <div class="card-body-content" class:busy-dim={state.busy}>
      {@render children()}
    </div>
  {/if}
</div>

<style>
  .card-body {
    position: relative;
    flex: 1;
    display: flex;
    flex-direction: column;
    min-height: 0;
  }

  /* a lone state block or the spinner fills and centres in a stretched card */
  .card-body-fill,
  .card-body > :global(.card-state) {
    flex: 1;
    display: flex;
    flex-direction: column;
    justify-content: center;
    align-items: center;
  }

  .card-body-spinner {
    display: inline-flex;
    color: var(--color-accent);
  }

  .card-body-content {
    transition: opacity var(--transition-fast);
  }
</style>
