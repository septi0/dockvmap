<script lang="ts">
  import Check from "@lucide/svelte/icons/check";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Modal from "./Modal.svelte";
  import Button from "./Button.svelte";
  import TagFamilyPicker from "./TagFamilyPicker.svelte";
  import RefreshTagsButton from "./RefreshTagsButton.svelte";
  import RefreshingIndicator from "./RefreshingIndicator.svelte";
  import {
    getImageTags,
    updateImageTag,
    markImageTagsAsSeen,
  } from "../api/images";
  import { errorMessage } from "../api/client";
  import type { TagGroup } from "../api/types/images";

  let {
    open,
    imageId,
    currentTag,
    refreshStatus = "idle",
    onClose,
    onTagUpdated,
    onTagsRefreshed,
  }: {
    open: boolean;
    imageId: number;
    currentTag: string;
    refreshStatus?: "idle" | "running";
    onClose: () => void;
    onTagUpdated: (tag: string) => void;
    onTagsRefreshed?: () => void;
  } = $props();

  let tagGroups = $state<TagGroup[]>([]);
  let loadingTags = $state(false);
  let tagsError = $state<string | null>(null);

  let selectedTag = $state<string | null>(null);

  let updating = $state(false);
  let updateError = $state<string | null>(null);
  let loadToken = 0;

  let wasRefreshing = false;

  const currentFamilyId = $derived(
    tagGroups.find((group) => group.tags.some((t) => t.tag === currentTag))
      ?.familyId ?? null,
  );
  const selectedFamilyId = $derived(
    selectedTag
      ? (tagGroups.find((group) =>
          group.tags.some((t) => t.tag === selectedTag),
        )?.familyId ?? null)
      : null,
  );
  const isFamilyMismatch = $derived(
    selectedTag !== null &&
      selectedTag !== currentTag &&
      currentFamilyId !== null &&
      selectedFamilyId !== null &&
      currentFamilyId !== selectedFamilyId,
  );

  async function loadTags() {
    const requestId = ++loadToken;
    loadingTags = true;
    tagsError = null;

    try {
      const result = await getImageTags(imageId);
      if (requestId !== loadToken) return;
      tagGroups = result.tagGroups;
    } catch (err) {
      if (requestId !== loadToken) return;
      tagsError = errorMessage(err, "Failed to load tags");
    } finally {
      if (requestId === loadToken) loadingTags = false;
    }
  }

  $effect(() => {
    if (!open) return;

    selectedTag = currentTag;
    updateError = null;
    loadTags();

    markImageTagsAsSeen(imageId).catch(() => {});
  });

  $effect(() => {
    const running = refreshStatus === "running";

    if (open && wasRefreshing && !running) {
      reloadTagsKeepingSelection();
    }

    wasRefreshing = running;
  });

  async function reloadTagsKeepingSelection() {
    await loadTags();

    const stillAvailable = tagGroups.some((group) =>
      group.tags.some((tagInfo) => tagInfo.tag === selectedTag),
    );
    if (!stillAvailable) selectedTag = currentTag;
  }

  async function handleTagsRefreshed(status: "refreshed" | "running") {
    if (status === "running") {
      onTagsRefreshed?.();
      return;
    }

    await reloadTagsKeepingSelection();
    onTagsRefreshed?.();
  }

  async function handleUpdate() {
    if (!selectedTag || selectedTag === currentTag) return;

    updateError = null;
    updating = true;

    try {
      await updateImageTag(imageId, selectedTag);
      onTagUpdated(selectedTag);
      onClose();
    } catch (err) {
      updateError = errorMessage(err, "Failed to update tag");
    } finally {
      updating = false;
    }
  }
</script>

<Modal {open} {onClose} title="Change tracked tag" size="lg">
  <div class="header-row">
    <p class="hint muted">
      Pick which upstream tag this virtual image should resolve to.
    </p>
    <div class="header-actions">
      {#if refreshStatus === "running"}
        <RefreshingIndicator text="Checking upstream…" />
      {:else}
        <RefreshTagsButton
          {imageId}
          onRefreshed={handleTagsRefreshed}
          label="Refresh"
        />
      {/if}
    </div>
  </div>

  <div class="current-banner">
    <Check size={14} strokeWidth={2.5} />
    <span>Currently tracking <strong>{currentTag}</strong></span>
  </div>

  {#if tagsError}
    <p class="error">
      <TriangleAlert size={16} strokeWidth={2} />
      {tagsError}
    </p>
  {:else if loadingTags}
    <p class="muted">Loading tags…</p>
  {:else}
    <div class="tags-scroll">
      <TagFamilyPicker
        {tagGroups}
        bind:selectedTag
        {currentTag}
        layout="stack"
        emptyMessage="No tags discovered yet. Try refreshing."
      />
    </div>
  {/if}

  {#if isFamilyMismatch}
    <p class="family-warning">
      <span class="icon"><TriangleAlert size={16} strokeWidth={2} /></span>
      <span class="text">
        <strong>{selectedTag}</strong> is from a different tag family than
        <strong>{currentTag}</strong>, so they might not be compatible for an
        in-place update.
      </span>
    </p>
  {/if}

  {#if updateError}
    <p class="error">
      <TriangleAlert size={16} strokeWidth={2} />
      {updateError}
    </p>
  {/if}

  <div class="tags-action-bar">
    <span class="selection">
      {#if selectedTag === currentTag}
        <span class="muted">No changes to apply</span>
      {:else}
        Switch to <span class="tag-value">{selectedTag}</span>?
      {/if}
    </span>
    <div class="tags-action-buttons">
      <Button variant="secondary" onclick={onClose}>Cancel</Button>
      <Button
        disabled={selectedTag === currentTag || updating}
        onclick={handleUpdate}
      >
        {updating ? "Updating…" : "Update tag"}
      </Button>
    </div>
  </div>
</Modal>

<style>
  .header-row {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-3);
    margin-bottom: var(--space-4);
  }

  .header-actions {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    flex-shrink: 0;
  }

  .hint {
    margin: 0;
    font-size: 0.875rem;
  }

  .current-banner {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-bottom: var(--space-4);
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    border: 1px solid color-mix(in srgb, var(--color-accent) 30%, transparent);
    background: var(--color-accent-muted-bg);
    color: var(--color-accent);
    font-size: 0.875rem;
  }

  .family-warning {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    margin-bottom: var(--space-4);
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    border: 1px solid color-mix(in srgb, var(--color-warning) 30%, transparent);
    background: var(--color-warning-bg);
    color: var(--color-warning);
    font-size: 0.8125rem;
    line-height: 1.4;
  }

  .family-warning .icon {
    display: inline-flex;
    flex-shrink: 0;
    margin-top: 1px;
  }

  .family-warning .text {
    min-width: 0;
  }

  .tags-scroll {
    max-height: 480px;
    overflow-y: auto;
    overscroll-behavior: contain;
    margin-bottom: var(--space-4);
    padding: 2px var(--space-3) 2px 2px;
    background-color: var(--color-surface);
    background-image:
      linear-gradient(var(--color-surface), var(--color-surface)),
      linear-gradient(to top, color-mix(in srgb, var(--color-text) 12%, transparent), transparent);
    background-repeat: no-repeat, no-repeat;
    background-position: bottom, bottom;
    background-size: 100% 28px, 100% 28px;
    /* local scrolls with content, masking the shadow only once actually scrolled to bottom */
    background-attachment: local, scroll;
  }

  .tags-action-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding-top: var(--space-4);
    border-top: 1px solid var(--color-border);
  }

  .tags-action-buttons {
    display: flex;
    gap: var(--space-2);
    flex-shrink: 0;
  }

  .selection {
    font-size: 0.875rem;
  }
</style>
