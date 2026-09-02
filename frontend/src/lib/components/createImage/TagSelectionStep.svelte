<script lang="ts">
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Button from "../Button.svelte";
  import TagFamilyPicker from "../TagFamilyPicker.svelte";
  import type { TagGroup } from "../../api/types/images";

  let {
    registryLabel,
    repository,
    tagGroups,
    tagCount,
    ignoredCount,
    selectedTag = $bindable(),
    creating,
    error,
    onBack,
    onCreate,
  }: {
    registryLabel: string | undefined;
    repository: string;
    tagGroups: TagGroup[];
    tagCount: number;
    ignoredCount: number;
    selectedTag: string | null;
    creating: boolean;
    error: string | null;
    onBack: () => void;
    onCreate: () => void;
  } = $props();
</script>

<p class="recap">
  <span class="recap-text">
    <strong>{registryLabel}</strong>/{repository}
  </span>
  <button type="button" class="link" onclick={onBack}>Edit</button>
</p>

<p class="discovery-summary">
  {tagCount}
  {tagCount === 1 ? "tag" : "tags"} found
  {#if ignoredCount > 0}
    &middot; {ignoredCount} filtered out
  {/if}
</p>

<TagFamilyPicker
  {tagGroups}
  bind:selectedTag
  emptyMessage="No tags were found for this repository."
/>

{#if error}
  <p class="error tag-error">
    <TriangleAlert size={16} strokeWidth={2} />
    {error}
  </p>
{/if}

<div class="create-bar">
  <span class="selection">
    {#if selectedTag}
      Selected tag: <span class="tag-value">{selectedTag}</span>
    {:else}
      <span class="muted">Pick a tag above to continue</span>
    {/if}
  </span>
  <Button disabled={!selectedTag || creating} onclick={onCreate}>
    {creating ? "Creating…" : "Create virtual image"}
  </Button>
</div>

<style>
  .recap {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    margin: 0 0 var(--space-5);
    padding-bottom: var(--space-4);
    border-bottom: 1px solid var(--color-border);
    font-size: 0.875rem;
  }

  .recap-text {
    font-family: monospace;
    color: var(--color-text-muted);
  }

  .discovery-summary {
    margin: 0 0 var(--space-4);
    font-size: 0.8125rem;
    color: var(--color-text-faint);
  }

  .link {
    flex-shrink: 0;
  }

  .create-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    margin-top: var(--space-5);
  }

  .tag-error {
    margin-top: var(--space-4);
  }

  .selection {
    font-size: 0.875rem;
  }
</style>
