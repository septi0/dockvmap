<script lang="ts">
  import type { Snippet } from "svelte";
  import LoaderCircle from "@lucide/svelte/icons/loader-circle";
  import BusyOverlay from "./BusyOverlay.svelte";

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
</script>

<div class="card-body">
  {#if loading}
    <div class="card-body-fill">
      <span class="card-body-spinner" aria-hidden="true">
        <LoaderCircle size={20} strokeWidth={3} />
      </span>
    </div>
  {:else if hasError && errorState}
    {@render errorState()}
  {:else if empty && emptyState}
    {@render emptyState()}
  {:else}
    <BusyOverlay active={busy} />
    <div class="card-body-content" class:is-busy={busy}>
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
    animation: spin 0.8s linear infinite;
  }

  .card-body-content {
    transition: opacity var(--transition-fast);
  }

  .card-body-content.is-busy {
    opacity: 0.55;
    pointer-events: none;
  }
</style>
