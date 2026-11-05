<script lang="ts">
  import ArrowLeftRight from "@lucide/svelte/icons/arrow-left-right";
  import ArrowUp from "@lucide/svelte/icons/arrow-up";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import DetailRow from "../DetailRow.svelte";
  import Button from "../Button.svelte";
  import TagChangeNotice from "../TagChangeNotice.svelte";
  import { allImages } from "../../utils/format";
  import type { Image } from "../../api/types/images";
  import type { ImageGroup } from "../../api/types/imageGroups";

  let {
    image,
    group,
    groupSize,
    lastChange,
    reverting,
    revertError,
    onChangeTag,
    onRevert,
  }: {
    image: Image;
    group: ImageGroup | null;
    groupSize: number;
    lastChange: { from: string; to: string } | null;
    reverting: boolean;
    revertError: string | null;
    onChangeTag: () => void;
    onRevert: () => void;
  } = $props();

  let updateAvailable = $derived(
    group ? group.updateAvailable : image.updateAvailable,
  );
  let updateAvailableTag = $derived(
    group ? group.updateAvailableTag : image.updateAvailableTag,
  );
  let updateBlocked = $derived(!!group && updateAvailable && !updateAvailableTag);
</script>

<DetailRow label="Current tag">
  <div class="detail-cell">
    <span class="detail-cell-row">
      <span class="tag-value current-tag">{image.tag}</span>
      {#if updateAvailable && !updateBlocked}
        <Button variant="warning" size="sm" onclick={onChangeTag}>
          <ArrowUp size={14} strokeWidth={2.5} />
          {group ? "Update group" : "Update"}
        </Button>
      {:else}
        <Button variant="secondary" size="sm" onclick={onChangeTag}>
          <ArrowLeftRight size={14} strokeWidth={2} />
          {group ? "Change group tag" : "Change tag"}
        </Button>
      {/if}
    </span>
    {#if lastChange && lastChange.to === image.tag}
      <TagChangeNotice
        from={lastChange.from}
        to={lastChange.to}
        {groupSize}
        disabled={reverting}
        {onRevert}
      />
    {/if}
    {#if revertError}
      <p class="error">
        <TriangleAlert size={16} strokeWidth={2} />
        {revertError}
      </p>
    {/if}
    {#if updateBlocked}
      <p class="warning-text">
        <TriangleAlert size={14} strokeWidth={2} />
        {#if group?.partialTag}
          Update blocked: {group.partialTag.tag} is missing from
          {group.partialTag.missing.join(", ")}.
        {:else}
          Update blocked: no newer tag is available on {allImages(groupSize)}.
        {/if}
      </p>
    {:else if updateAvailable && !updateAvailableTag}
      <p class="warning-text">
        <TriangleAlert size={14} strokeWidth={2} />
        A newer tag is available in this tag's family.
      </p>
    {/if}
  </div>
</DetailRow>

<style>
  .current-tag {
    padding: 2px var(--space-3);
    border: 1px solid var(--color-border-strong);
    border-radius: 999px;
    background: var(--color-surface-hover);
    color: var(--color-text);
    font-size: 0.9375rem;
  }
</style>
