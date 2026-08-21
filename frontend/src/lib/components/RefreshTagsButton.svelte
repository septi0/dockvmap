<script lang="ts">
  import RefreshCw from "@lucide/svelte/icons/refresh-cw";
  import Button from "./Button.svelte";
  import ConfirmDialog from "./ConfirmDialog.svelte";
  import { refreshImageTags } from "../api/images";
  import { ApiError } from "../api/client";
  import { toast } from "../services/toast";

  let {
    imageId,
    onRefreshed,
    label = "Refresh tags",
    variant = "link",
  }: {
    imageId: number;
    onRefreshed?: () => void | Promise<void>;
    label?: string;
    variant?: "link" | "button";
  } = $props();

  let showConfirm = $state(false);
  let refreshing = $state(false);
  let error = $state<string | null>(null);

  function cancel() {
    showConfirm = false;
    error = null;
  }

  async function confirm() {
    error = null;
    refreshing = true;

    try {
      await refreshImageTags(imageId);
      showConfirm = false;
      await onRefreshed?.();
      toast.success("Tags refreshed.");
    } catch (err) {
      error = err instanceof ApiError ? err.message : "Failed to refresh tags";
    } finally {
      refreshing = false;
    }
  }
</script>

{#if variant === "button"}
  <Button variant="secondary" size="sm" onclick={() => (showConfirm = true)}>
    <RefreshCw size={14} strokeWidth={2} />
    {label}
  </Button>
{:else}
  <button
    type="button"
    class="text-button"
    onclick={() => (showConfirm = true)}
  >
    <span class="icon"><RefreshCw size={14} strokeWidth={2} /></span>
    {label}
  </button>
{/if}

<ConfirmDialog
  open={showConfirm}
  title="Refresh tags"
  message="Check the upstream registry for new or updated tags now?"
  confirmLabel="Refresh"
  {error}
  submitting={refreshing}
  onConfirm={confirm}
  onCancel={cancel}
/>

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

  .icon {
    display: inline-flex;
  }
</style>
