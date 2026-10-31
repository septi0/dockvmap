<script lang="ts">
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import type { ImageGroup } from "../../api/types/imageGroups";

  let {
    group,
    blocked,
    detailsQuery,
  }: {
    group: ImageGroup;
    blocked: boolean;
    detailsQuery: string;
  } = $props();
</script>

<div class="members">
  {#each group.images as member (member.id)}
    {@const missing =
      blocked && group.partialTag?.missing.includes(member.name)}
    <a
      class="member"
      class:missing
      href={`#/images/${member.id}${detailsQuery}`}
    >
      {#if missing}
        <TriangleAlert size={12} strokeWidth={2} />
      {/if}
      {member.name}
    </a>
  {/each}
</div>

<style>
  .members {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .member {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    max-width: 100%;
    overflow-wrap: break-word;
    padding: 1px var(--space-2);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    background: var(--color-bg);
    color: var(--color-text-muted);
    font-size: 0.8125rem;
    text-decoration: none;
  }

  .member:hover {
    border-color: var(--color-border-strong);
    color: var(--color-text);
  }

  .member.missing {
    border-color: var(--color-warning);
    background: var(--color-warning-bg);
    color: var(--color-warning);
  }
</style>
