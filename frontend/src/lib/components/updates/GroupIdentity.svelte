<script lang="ts">
  import Layers from "@lucide/svelte/icons/layers";
  import type { ImageGroup } from "../../api/types/imageGroups";

  let {
    group,
    groupSize,
    onOpenGroup,
  }: {
    group: ImageGroup;
    groupSize: number;
    onOpenGroup: () => void;
  } = $props();
</script>

<span class="group-icon"><Layers size={16} strokeWidth={2} /></span>
<button
  type="button"
  class="group-link"
  onclick={onOpenGroup}>{group.name}</button
>
<span class="badge">Group · {groupSize} images</span>

<style>
  .group-link {
    padding: 0;
    border: none;
    background: none;
    color: inherit;
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
