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
  }: {
    imageId: number;
    onRefreshed?: (
      status: "refreshed" | "running",
    ) => void | Promise<void>;
    label?: string;
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

<Button variant="secondary" size="sm" disabled={refreshing} onclick={refresh}>
  <span class="icon" class:spin={refreshing}><RefreshCw size={14} strokeWidth={2} /></span>
  {label}
</Button>

{#if error}
  <span class="refresh-error">{error}</span>
{/if}

<style>
  .icon {
    display: inline-flex;
  }

  .refresh-error {
    font-size: 0.8125rem;
    color: var(--color-danger);
  }
</style>
