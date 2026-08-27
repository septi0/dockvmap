<script lang="ts">
  import Check from "@lucide/svelte/icons/check";
  import type { TagGroup } from "../api/types/images";

  let {
    tagGroups,
    selectedTag = $bindable(null),
    currentTag = null,
    visiblePerFamily = 10,
    layout = "stack",
    emptyMessage = "No tags were found.",
  }: {
    tagGroups: TagGroup[];
    selectedTag?: string | null;
    currentTag?: string | null;
    visiblePerFamily?: number;
    layout?: "stack" | "grid";
    emptyMessage?: string;
  } = $props();

  let expandedFamilies = $state<Set<number>>(new Set());

  $effect(() => {
    tagGroups;
    expandedFamilies = new Set();
  });

  function expandFamily(familyId: number) {
    expandedFamilies = new Set(expandedFamilies).add(familyId);
  }

  function visibleCountFor(group: TagGroup): number {
    if (currentTag) {
      const index = group.tags.findIndex(
        (tagInfo) => tagInfo.tag === currentTag,
      );
      if (index !== -1) return Math.max(visiblePerFamily, index + 1);
    }

    return visiblePerFamily;
  }
</script>

{#if tagGroups.length === 0}
  <p class="muted">{emptyMessage}</p>
{:else}
  <div class="families" class:grid={layout === "grid"}>
    {#each tagGroups as group (group.familyId)}
      {@const visibleCount = visibleCountFor(group)}
      {@const isExpanded = expandedFamilies.has(group.familyId)}
      {@const visibleTags = isExpanded
        ? group.tags
        : group.tags.slice(0, visibleCount)}
      {@const currentIndex = currentTag
        ? group.tags.findIndex((t) => t.tag === currentTag)
        : -1}
      {@const isCurrentFamily = currentIndex !== -1}
      <div class="family" class:current={isCurrentFamily}>
        <div class="tags">
          {#each visibleTags as tagInfo (tagInfo.tag)}
            <button
              type="button"
              class="tag-chip"
              class:selected={selectedTag === tagInfo.tag}
              onclick={() => (selectedTag = tagInfo.tag)}
            >
              {#if tagInfo.tag === currentTag}
                <Check size={12} strokeWidth={3} />
              {/if}
              {tagInfo.tag}
              {#if tagInfo.new}
                <span class="new-dot" title="New since last check"></span>
              {/if}
            </button>
          {/each}
          {#if !isExpanded && group.tags.length > visibleCount}
            <button
              type="button"
              class="show-more"
              onclick={() => expandFamily(group.familyId)}
            >
              +{group.tags.length - visibleCount} more
            </button>
          {/if}
        </div>
      </div>
    {/each}
  </div>
{/if}

<style>
  .families {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .families.grid {
    display: block;
    columns: 300px 3;
    column-gap: var(--space-4);
  }

  .families.grid .family {
    break-inside: avoid;
    margin-bottom: var(--space-4);
  }

  .family {
    padding: var(--space-4);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
  }

  .family.current {
    border-color: var(--color-accent);
  }

  .tags {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .tag-chip {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    padding: var(--space-1) var(--space-3);
    border-radius: var(--radius-full);
    border: 1px solid var(--color-border-strong);
    background: var(--color-surface);
    color: var(--color-text);
    font-size: 0.8125rem;
    font-family: monospace;
    cursor: pointer;
  }

  .tag-chip:hover {
    border-color: var(--color-accent);
  }

  .tag-chip.selected {
    border-color: var(--color-accent);
    background: var(--color-accent-muted-bg);
    color: var(--color-accent);
  }

  .new-dot {
    width: 6px;
    height: 6px;
    border-radius: var(--radius-full);
    background: var(--color-accent);
  }

  .show-more {
    padding: var(--space-1) var(--space-3);
    border-radius: var(--radius-full);
    border: 1px dashed var(--color-border-strong);
    background: none;
    color: var(--color-text-muted);
    font-size: 0.8125rem;
    cursor: pointer;
  }

  .show-more:hover {
    color: var(--color-text);
    border-color: var(--color-text-faint);
  }
</style>
