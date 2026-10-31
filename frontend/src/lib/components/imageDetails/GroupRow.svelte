<script lang="ts">
  import { link } from "svelte-spa-router";
  import Layers from "@lucide/svelte/icons/layers";
  import Plus from "@lucide/svelte/icons/plus";
  import Settings2 from "@lucide/svelte/icons/settings-2";
  import DetailRow from "../DetailRow.svelte";
  import Button from "../Button.svelte";
  import GroupPullNotice from "../GroupPullNotice.svelte";
  import type { ImageGroup } from "../../api/types/imageGroups";

  let {
    group,
    groupSize,
    onManage,
  }: {
    group: ImageGroup | null;
    groupSize: number;
    onManage: () => void;
  } = $props();
</script>

<DetailRow label="Group">
  <div class="detail-cell">
    <span class="detail-cell-row">
      {#if group}
        <span class="group-name">
          <Layers size={14} strokeWidth={2} />
          {group.name}
        </span>
        <a
          class="group-size"
          href={`/images?group=${group.id}`}
          use:link>{groupSize} images</a
        >
        <Button variant="secondary" size="sm" onclick={onManage}>
          <Settings2 size={14} strokeWidth={2} />
          Manage group
        </Button>
      {:else}
        <span class="muted">Not in a group</span>
        <Button variant="secondary" size="sm" onclick={onManage}>
          <Plus size={14} strokeWidth={2} />
          Add to group
        </Button>
      {/if}
    </span>
    {#if group}
      <GroupPullNotice {group} showNames />
    {/if}
  </div>
</DetailRow>

<style>
  .group-name {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    font-weight: 500;
  }

  .group-size {
    color: var(--color-text-muted);
    font-size: 0.8125rem;
  }

  .group-size:hover {
    color: var(--color-text);
  }
</style>
