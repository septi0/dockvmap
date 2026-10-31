<script lang="ts">
  import RefreshCw from "@lucide/svelte/icons/refresh-cw";
  import Button from "./Button.svelte";
  import { refreshImageTags } from "../api/images";
  import { errorMessage } from "../api/client";
  import { toast } from "../services/toast";

  let {
    imageId,
    onRefreshed,
    label = "Refresh tags",
    variant = "link",
  }: {
    imageId: number;
    onRefreshed?: (
      status: "refreshed" | "running",
    ) => void | Promise<void>;
    label?: string;
    variant?: "link" | "button";
  } = $props();

  let refreshing = $state(false);
  let error = $state<string | null>(null);

  async function refresh() {
    error = null;
    refreshing = true;

    try {
      const res = await refreshImageTags(imageId);

      if (res.status === "error") {
        error = res.error ?? "Failed to refresh tags";
        return;
      }

      await onRefreshed?.(res.status);

      toast.success(
        res.status === "running"
          ? "Refresh started - running in the background."
          : "Tags refreshed.",
      );
    } catch (err) {
      error = errorMessage(err, "Failed to refresh tags");
    } finally {
      refreshing = false;
    }
  }
</script>

{#if variant === "button"}
  <Button variant="secondary" size="sm" disabled={refreshing} onclick={refresh}>
    <span class="icon" class:spin={refreshing}><RefreshCw size={14} strokeWidth={2} /></span>
    {label}
  </Button>
{:else}
  <button type="button" class="text-button" disabled={refreshing} onclick={refresh}>
    <span class="icon" class:spin={refreshing}><RefreshCw size={14} strokeWidth={2} /></span>
    {label}
  </button>
{/if}

{#if error}
  <span class="refresh-error">{error}</span>
{/if}

<style>
  .text-button {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    background: none;
    border: none;
    padding: 0;
    font: inherit;
    font-size: 0.8125rem;
    color: var(--color-accent);
    cursor: pointer;
  }

  .text-button:disabled {
    cursor: default;
    opacity: 0.6;
  }

  .icon {
    display: inline-flex;
  }

  .refresh-error {
    font-size: 0.8125rem;
    color: var(--color-danger);
  }
</style>
