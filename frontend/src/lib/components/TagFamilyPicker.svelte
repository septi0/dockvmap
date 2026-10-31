<script lang="ts">
  import Check from "@lucide/svelte/icons/check";
  import { formatDate, formatRelativeTime } from "../utils/format";
  import type { Tag, TagGroup } from "../api/types/images";

  const SEARCH_MIN_TAGS = 12;

  let {
    tagGroups,
    selectedTag = $bindable(null),
    currentTag = null,
    visiblePerFamily = 10,
    layout = "stack",
    emptyMessage = "No tags were found.",
    dimPrereleases = false,
    searchable = false,
    showAge = false,
  }: {
    tagGroups: TagGroup[];
    selectedTag?: string | null;
    currentTag?: string | null;
    visiblePerFamily?: number;
    layout?: "stack" | "grid";
    emptyMessage?: string;
    dimPrereleases?: boolean;
    searchable?: boolean;
    showAge?: boolean;
  } = $props();

  let expandedFamilies = $state<Set<number>>(new Set());
  let search = $state("");

  let term = $derived(search.trim().toLowerCase());
  let tagTotal = $derived(
    tagGroups.reduce((total, group) => total + group.tags.length, 0),
  );
  let showSearch = $derived(searchable && tagTotal >= SEARCH_MIN_TAGS);
  let visibleGroups = $derived(
    term
      ? tagGroups
          .map((group) => ({
            ...group,
            tags: group.tags.filter((t) => t.tag.toLowerCase().includes(term)),
          }))
          .filter((group) => group.tags.length > 0)
      : tagGroups,
  );
  let currentFamilyId = $derived(
    tagGroups.find((group) => group.tags.some((t) => t.tag === currentTag))
      ?.familyId,
  );
  let firstTrackedAt = $derived(
    Math.min(
      ...tagGroups.flatMap((group) =>
        group.tags.map((t) => seenTime(t) ?? Infinity),
      ),
    ),
  );

  function seenTime(tag: Tag): number | null {
    const time = tag.firstSeen ? new Date(tag.firstSeen).getTime() : NaN;

    return Number.isNaN(time) ? null : time;
  }

  function seenSinceTracking(tag: Tag): boolean {
    const time = seenTime(tag);

    return time !== null && time > firstTrackedAt;
  }

  function ageTitle(tag: Tag): string {
    return seenSinceTracking(tag)
      ? `First seen ${formatDate(tag.firstSeen)}`
      : "Already published when tracking started";
  }

  function ignoreEnter(event: KeyboardEvent) {
    if (event.key === "Enter") event.preventDefault();
  }

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

{#if showSearch}
  <div class="tag-search">
    <input
      type="search"
      class="input"
      aria-label="Search tags"
      placeholder={`Search ${tagTotal.toLocaleString()} tags…`}
      bind:value={search}
      onkeydown={ignoreEnter}
    />
  </div>
{/if}

{#if tagGroups.length === 0}
  <p class="muted">{emptyMessage}</p>
{:else if visibleGroups.length === 0}
  <p class="muted">No tags match "{search.trim()}".</p>
{:else}
  <div class="families" class:grid={layout === "grid"}>
    {#each visibleGroups as group (group.familyId)}
      {@const visibleCount = visibleCountFor(group)}
      {@const isExpanded = expandedFamilies.has(group.familyId)}
      {@const showAll = isExpanded || term !== ""}
      {@const visibleTags = showAll
        ? group.tags
        : group.tags.slice(0, visibleCount)}
      <div class="family" class:current={group.familyId === currentFamilyId}>
        <div class="tags">
          {#each visibleTags as tagInfo (tagInfo.tag)}
            <button
              type="button"
              class="tag-chip"
              class:selected={selectedTag === tagInfo.tag}
              class:dimmed={dimPrereleases && tagInfo.prerelease}
              title={showAge ? ageTitle(tagInfo) : undefined}
              onclick={() => (selectedTag = tagInfo.tag)}
            >
              {#if tagInfo.tag === currentTag}
                <Check size={12} strokeWidth={3} />
              {/if}
              {tagInfo.tag}
              {#if showAge && seenSinceTracking(tagInfo)}
                <span class="tag-age">
                  {formatRelativeTime(tagInfo.firstSeen)}
                </span>
              {/if}
              {#if tagInfo.new}
                <span class="new-dot" title="New since last check"></span>
              {/if}
            </button>
          {/each}
          {#if !showAll && group.tags.length > visibleCount}
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
  .tag-search {
    position: sticky;
    top: 0;
    z-index: 1;
    margin-bottom: var(--space-3);
    background: var(--color-surface);
  }

  .tag-age {
    font-family: inherit;
    font-size: 0.6875rem;
    color: var(--color-text-muted);
    white-space: nowrap;
  }

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

  .tag-chip.dimmed:not(.selected) {
    opacity: 0.55;
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
