<script lang="ts">
  import Pin from "@lucide/svelte/icons/pin";
  import PinOff from "@lucide/svelte/icons/pin-off";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import DetailRow from "../DetailRow.svelte";
  import Button from "../Button.svelte";
  import { allImages } from "../../utils/format";
  import type { ImageGroup } from "../../api/types/imageGroups";

  let {
    group,
    groupSize,
    pinned,
    pinning,
    pinError,
    onTogglePin,
  }: {
    group: ImageGroup | null;
    groupSize: number;
    pinned: boolean;
    pinning: boolean;
    pinError: string | null;
    onTogglePin: () => void;
  } = $props();

  let scopeLabel = $derived(group ? "group" : "image");
</script>

<DetailRow label="Update tracking">
  <div class="detail-cell">
    <span class="detail-cell-row">
      {#if pinned}
        <span class="tracking-state is-pinned">
          <Pin size={14} strokeWidth={2} />
          Pinned to the current tag
        </span>
      {:else}
        <span
          class="tracking-state"
          title="Newer tags in this tag's family are flagged as an available update and included in tag notification emails."
          >Tracking new versions</span
        >
      {/if}
      <Button
        variant="secondary"
        size="sm"
        disabled={pinning}
        onclick={onTogglePin}
      >
        {#if pinned}
          <PinOff size={14} strokeWidth={2} />
          Unpin {scopeLabel}
        {:else}
          <Pin size={14} strokeWidth={2} />
          Pin {scopeLabel}
        {/if}
      </Button>
    </span>
    {#if pinned}
      <p class="tracking-hint muted">
        Upstream tags are still fetched, but newer versions aren't flagged
        as an available update or sent in tag notification emails{group
          ? `, for ${allImages(groupSize)} in this group`
          : ""}.
      </p>
    {/if}
    {#if pinError}
      <p class="error">
        <TriangleAlert size={16} strokeWidth={2} />
        {pinError}
      </p>
    {/if}
  </div>
</DetailRow>

<style>
  .tracking-state {
    font-weight: 500;
  }

  .tracking-state.is-pinned {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    font-weight: 600;
    color: var(--color-success);
  }

  .tracking-hint {
    margin: 0;
    font-size: 0.8125rem;
  }
</style>
