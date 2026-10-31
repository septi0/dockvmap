<script lang="ts">
  import Pin from "@lucide/svelte/icons/pin";
  import PinOff from "@lucide/svelte/icons/pin-off";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import DetailRow from "../DetailRow.svelte";
  import Button from "../Button.svelte";
  import { allImages } from "../../utils/format";
  import type { Image } from "../../api/types/images";
  import type { ImageGroup } from "../../api/types/imageGroups";

  let {
    image,
    group,
    groupSize,
    pinned,
    pinning,
    pinError,
    onTogglePin,
  }: {
    image: Image;
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
      <span class="tracking-state" class:is-pinned={pinned}>
        {#if pinned}
          <Pin size={14} strokeWidth={2} />
          Pinned to the current tag
        {:else}
          Tracking new versions
        {/if}
      </span>
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
    <p class="tracking-hint muted">
      {#if group}
        Pinning applies to {allImages(groupSize)} in this group.
      {:else if image.pinned}
        Upstream tags are still fetched, but newer versions aren't
        flagged as an available update or sent in tag notification
        emails.
      {:else}
        Newer tags in this tag's family are flagged as an available
        update and included in tag notification emails.
      {/if}
    </p>
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
