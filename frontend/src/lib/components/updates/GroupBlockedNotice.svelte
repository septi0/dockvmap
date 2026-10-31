<script lang="ts">
  import Settings2 from "@lucide/svelte/icons/settings-2";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Button from "../Button.svelte";
  import { allImages } from "../../utils/format";
  import type { ImageGroup } from "../../api/types/imageGroups";

  let {
    group,
    groupSize,
    disabled,
    onManage,
  }: {
    group: ImageGroup | null;
    groupSize: number;
    disabled: boolean;
    onManage: () => void;
  } = $props();
</script>

<p class="notice warning">
  <TriangleAlert size={14} strokeWidth={2} />
  <span class="notice-text">
    <strong>Update blocked.</strong>
    {#if group?.partialTag}
      <span class="tag-name">{group.partialTag.tag}</span> is on
      {groupSize - group.partialTag.missing.length} of {groupSize} images.
      <strong>{group.partialTag.missing.join(", ")}</strong>
      {group.partialTag.missing.length === 1 ? "doesn't" : "don't"} have it.
      Remove {group.partialTag.missing.length === 1 ? "that image" : "those images"}
      from the group to continue, or wait for the release to reach
      {group.partialTag.missing.length === 1 ? "it" : "them"}.
    {:else}
      No newer tag is available on {allImages(groupSize)}.
    {/if}
  </span>
  <Button variant="secondary" size="sm" {disabled} onclick={onManage}>
    <Settings2 size={14} strokeWidth={2} />
    Manage group
  </Button>
</p>

<style>
  .tag-name {
    font-family: monospace;
  }
</style>
