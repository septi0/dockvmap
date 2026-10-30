<script lang="ts">
  import { onDestroy, onMount, untrack } from "svelte";
  import Pin from "@lucide/svelte/icons/pin";
  import PinOff from "@lucide/svelte/icons/pin-off";
  import Undo2 from "@lucide/svelte/icons/undo-2";
  import Check from "@lucide/svelte/icons/check";
  import ArrowUp from "@lucide/svelte/icons/arrow-up";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import AsyncState from "./AsyncState.svelte";
  import Button from "./Button.svelte";
  import ChangeTagModal from "./ChangeTagModal.svelte";
  import RefreshingIndicator from "./RefreshingIndicator.svelte";
  import TagFamilyPicker from "./TagFamilyPicker.svelte";
  import {
    getImage,
    getNewerImageTags,
    markImageTagsAsSeen,
    setImagePin,
    updateImageTag,
  } from "../api/images";
  import { errorMessage } from "../api/client";
  import { createImageRefresh } from "../services/imageRefresh";
  import { updatesCount } from "../services/updatesCount";
  import type { Image, TagGroup } from "../api/types/images";

  let {
    image,
    detailsQuery,
    sweepRunning,
    reloadToken,
    onImageChanged,
  }: {
    image: Image;
    detailsQuery: string;
    sweepRunning: boolean;
    reloadToken: number;
    onImageChanged: (image: Image) => void;
  } = $props();

  let tagGroups = $state<TagGroup[]>([]);
  let selectedTag = $state<string | null>(null);
  let tagsLoaded = $state(false);
  let tagsError = $state<string | null>(null);
  let tagsToken = 0;

  let working = $state(false);
  let actionError = $state<string | null>(null);
  let lastChange = $state<{ from: string; to: string } | null>(null);

  let showTagModal = $state(false);
  let refreshStatus = $state<"idle" | "running" | "stale">("idle");

  let locked = $derived(sweepRunning || refreshStatus === "running");
  let hasNewSelection = $derived(!!selectedTag && selectedTag !== image.tag);

  const imageRefresh = createImageRefresh(() => image.id, {
    onResult: (fresh) => onImageChanged(fresh),
  });

  const stopRefreshSubscription = imageRefresh.subscribe(
    (status) => (refreshStatus = status),
  );
  const stopSettledListener = imageRefresh.onSettled(loadTags);

  function hasTag(tag: string | null | undefined) {
    return tagGroups.some((group) => group.tags.some((t) => t.tag === tag));
  }

  async function loadTags() {
    const requestId = ++tagsToken;
    tagsError = null;

    try {
      const result = await getNewerImageTags(image.id);
      if (requestId !== tagsToken) return;

      tagGroups = result.tagGroups;

      if (!hasTag(selectedTag)) selectedTag = image.tag;
    } catch (err) {
      if (requestId !== tagsToken) return;
      tagsError = errorMessage(err, "Failed to load tags");
    } finally {
      if (requestId === tagsToken) tagsLoaded = true;
    }
  }

  async function reloadRow() {
    try {
      const fresh = await getImage(image.id);
      onImageChanged(fresh);
      imageRefresh.sync(fresh);
    } catch (err) {
      tagsError = errorMessage(err, "Failed to reload image");
      return;
    }

    await loadTags();
  }

  onMount(() => {
    imageRefresh.sync(image);
    loadTags();
  });

  $effect(() => {
    if (reloadToken > 0) untrack(reloadRow);
  });

  onDestroy(() => {
    stopRefreshSubscription();
    stopSettledListener();
    imageRefresh.destroy();
  });

  async function settleTagChange(from: string, to: string) {
    lastChange = { from, to };
    selectedTag = null;
    markImageTagsAsSeen(image.id).catch(() => {});
    updatesCount.refresh();
    await reloadRow();
  }

  async function applySelectedTag() {
    if (!selectedTag || !hasNewSelection) return;

    const from = image.tag;
    const to = selectedTag;

    actionError = null;
    working = true;

    try {
      await updateImageTag(image.id, to);
      await settleTagChange(from, to);
    } catch (err) {
      actionError = errorMessage(err, "Failed to update tag");
    } finally {
      working = false;
    }
  }

  async function revert() {
    if (!lastChange) return;

    actionError = null;
    working = true;

    try {
      await updateImageTag(image.id, lastChange.from, "restore");
      lastChange = null;
      selectedTag = null;
      updatesCount.refresh();
      await reloadRow();
    } catch (err) {
      actionError = errorMessage(err, "Failed to revert tag");
    } finally {
      working = false;
    }
  }

  async function setPinned(pinned: boolean) {
    actionError = null;
    working = true;

    try {
      await setImagePin(image.id, pinned);
      updatesCount.refresh();
      await reloadRow();
    } catch (err) {
      actionError = errorMessage(err, "Failed to update pin state");
    } finally {
      working = false;
    }
  }
</script>

<article class="card update-row">
  <header class="row-head">
    <div class="identity">
      <a class="row-link" href={`#/images/${image.id}${detailsQuery}`}
        >{image.name}</a
      >
      <span class="muted source">{image.registry}/{image.repository}</span>
    </div>
    <span class="versions muted">
      <span>Current <span class="tag-value">{image.tag}</span></span>
      {#if image.updateAvailable && image.updateAvailableTag}
        <span>Suggested <strong>{image.updateAvailableTag}</strong></span>
      {/if}
    </span>
  </header>

  {#if lastChange}
    <p class="notice">
      <Check size={14} strokeWidth={2.5} />
      <span class="notice-text">
        Updated from <strong>{lastChange.from}</strong> to
        <strong>{lastChange.to}</strong>
      </span>
      <Button
        variant="secondary"
        size="sm"
        disabled={working || locked}
        onclick={revert}
      >
        <Undo2 size={14} strokeWidth={2} />
        Revert
      </Button>
    </p>
  {/if}

  {#if image.pinned}
    <p class="notice">
      <Pin size={14} strokeWidth={2} />
      <span class="notice-text">
        Pinned to <strong>{image.tag}</strong>. New versions won't be flagged.
      </span>
      <Button
        variant="secondary"
        size="sm"
        disabled={working || locked}
        onclick={() => setPinned(false)}
      >
        <PinOff size={14} strokeWidth={2} />
        Unpin
      </Button>
    </p>
  {:else if image.updateAvailable}
    <AsyncState
      loading={!tagsLoaded}
      error={tagsError}
      empty={tagGroups.length === 0}
      emptyMessage="No newer tags in this family."
      emptyVariant="text"
    >
      <TagFamilyPicker
        {tagGroups}
        bind:selectedTag
        currentTag={image.tag}
        dimPrereleases
      />
    </AsyncState>

    <div class="row-actions">
      {#if locked}
        <RefreshingIndicator text="Checking upstream…" />
      {/if}
      <Button
        variant="secondary"
        size="sm"
        disabled={working}
        onclick={() => (showTagModal = true)}
      >
        All tags…
      </Button>
      <Button
        variant="secondary"
        size="sm"
        disabled={working || locked}
        onclick={() => setPinned(true)}
      >
        <Pin size={14} strokeWidth={2} />
        Pin
      </Button>
      <Button
        size="sm"
        disabled={!hasNewSelection || working || locked}
        onclick={applySelectedTag}
      >
        <ArrowUp size={14} strokeWidth={2.5} />
        {hasNewSelection ? `Update to ${selectedTag}` : "Select a new tag"}
      </Button>
    </div>
  {:else if !lastChange}
    <p class="muted empty-note">No update is flagged for this image anymore.</p>
  {/if}

  {#if actionError}
    <p class="error">
      <TriangleAlert size={16} strokeWidth={2} />
      {actionError}
    </p>
  {/if}
</article>

{#if showTagModal}
  <ChangeTagModal
    open
    imageId={image.id}
    currentTag={image.tag}
    refreshStatus={refreshStatus === "stale" ? "idle" : refreshStatus}
    onClose={() => (showTagModal = false)}
    onTagUpdated={(tag) => settleTagChange(image.tag, tag)}
    onTagsRefreshed={reloadRow}
  />
{/if}

<style>
  .update-row {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-4);
    margin-bottom: var(--space-3);
  }

  .update-row :global(.tag-chip) {
    max-width: 100%;
    overflow-wrap: break-word;
  }

  .row-head {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-1) var(--space-3);
  }

  .identity {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--space-1) var(--space-2);
    min-width: 0;
    max-width: 100%;
  }

  .row-link {
    color: inherit;
    text-decoration: none;
    font-weight: 500;
  }

  .row-link:hover {
    text-decoration: underline;
  }

  .source,
  .versions,
  .versions > span {
    min-width: 0;
    max-width: 100%;
    font-size: 0.8125rem;
    overflow-wrap: break-word;
  }

  .versions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1) var(--space-3);
  }

  .versions strong {
    font-family: monospace;
    color: var(--color-text);
  }

  .notice {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    margin: 0;
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    background: var(--color-accent-muted-bg);
    color: var(--color-accent);
    font-size: 0.875rem;
  }

  .notice-text {
    flex: 1;
    min-width: 0;
    overflow-wrap: break-word;
  }

  .row-actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: flex-end;
    gap: var(--space-2);
  }

  .empty-note {
    margin: 0;
    font-size: 0.875rem;
  }
</style>
