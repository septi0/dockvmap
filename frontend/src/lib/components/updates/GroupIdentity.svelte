<script lang="ts">
  import Layers from "@lucide/svelte/icons/layers";
  import UpstreamLink from "../UpstreamLink.svelte";
  import type { Image } from "../../api/types/images";
  import type { ImageGroup } from "../../api/types/imageGroups";

  let {
    group,
    image,
    groupSize,
    onOpenGroup,
  }: {
    group: ImageGroup;
    image: Image;
    groupSize: number;
    onOpenGroup: () => void;
  } = $props();
</script>

<span class="group-icon"><Layers size={16} strokeWidth={2} /></span>
<button
  type="button"
  class="row-link group-link"
  onclick={onOpenGroup}>{group.name}</button
>
<span class="badge">Group · {groupSize} images</span>
<UpstreamLink registry={image.registry} repository={image.repository} />

<style>
  .group-link {
    padding: 0;
    border: none;
    background: none;
    font: inherit;
    font-weight: 500;
    cursor: pointer;
  }

  .group-icon {
    display: inline-flex;
    align-self: center;
    color: var(--color-text-muted);
  }
</style>
