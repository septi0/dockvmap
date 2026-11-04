<script lang="ts">
  import { onDestroy, onMount, untrack } from "svelte";
  import Pin from "@lucide/svelte/icons/pin";
  import PinOff from "@lucide/svelte/icons/pin-off";
  import Tags from "@lucide/svelte/icons/tags";
  import ArrowUp from "@lucide/svelte/icons/arrow-up";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import AsyncState from "./AsyncState.svelte";
  import Button from "./Button.svelte";
  import ChangeTagModal from "./ChangeTagModal.svelte";
  import ImageGroupModal from "./ImageGroupModal.svelte";
  import RefreshingIndicator from "./RefreshingIndicator.svelte";
  import TagChangeNotice from "./TagChangeNotice.svelte";
  import TagFamilyPicker from "./TagFamilyPicker.svelte";
  import UpstreamLink from "./UpstreamLink.svelte";
  import GroupBlockedNotice from "./updates/GroupBlockedNotice.svelte";
  import GroupIdentity from "./updates/GroupIdentity.svelte";
  import GroupMembers from "./updates/GroupMembers.svelte";
  import {
    getImage,
    getNewerImageTags,
    markImageTagsAsSeen,
    setImagePin,
    updateImageTag,
  } from "../api/images";
  import { getImageGroup } from "../api/imageGroups";
  import { errorMessage } from "../api/client";
  import { allCount, allImages } from "../utils/format";
  import { createImageRefresh } from "../services/imageRefresh";
  import { updatesCount } from "../services/updatesCount";
  import type { Image, TagGroup } from "../api/types/images";
  import type { ImageGroup } from "../api/types/imageGroups";
  import { imageSource } from "../utils/registries";

  let {
    image,
    group = null,
    detailsQuery,
    sweepRunning,
    reloadToken,
    onImageChanged,
    onGroupChanged,
    onStructureChanged,
  }: {
    image: Image;
    group?: ImageGroup | null;
    detailsQuery: string;
    sweepRunning: boolean;
    reloadToken: number;
    onImageChanged: (image: Image) => void;
    onGroupChanged?: (group: ImageGroup) => void;
    onStructureChanged?: () => void;
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
  let showGroupModal = $state(false);
  let refreshStatus = $state<"idle" | "running" | "stale">("idle");

  let locked = $derived(sweepRunning || refreshStatus === "running");
  let hasNewSelection = $derived(!!selectedTag && selectedTag !== image.tag);
  let groupSize = $derived(group?.images.length ?? 0);
  let pinned = $derived(group ? group.pinned : image.pinned);
  let flagged = $derived(group ? group.updateAvailable : image.updateAvailable);
  let suggestedTag = $derived(
    group ? group.updateAvailableTag : image.updateAvailableTag,
  );
  let blocked = $derived(!!group && flagged && !suggestedTag);

  const imageRefresh = createImageRefresh(() => image.id, {
    onResult: (fresh) => onImageChanged(fresh),
  });

  const stopRefreshSubscription = imageRefresh.subscribe(
    (status) => (refreshStatus = status),
  );
  const stopSettledListener = imageRefresh.onSettled(() =>
    group ? reloadRow() : loadTags(),
  );

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
      if (group) {
        const fresh = await getImageGroup(group.id);
        onGroupChanged?.(fresh);
        imageRefresh.sync(fresh.images[0]);
      } else {
        const fresh = await getImage(image.id);
        onImageChanged(fresh);
        imageRefresh.sync(fresh);
      }
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
      actionError = errorMessage(err, "Failed to roll back");
    } finally {
      working = false;
    }
  }

  async function handleGroupSaved() {
    updatesCount.refresh();
    await reloadRow();
    onStructureChanged?.();
  }

  function handleGroupDeleted() {
    updatesCount.refresh();
    onStructureChanged?.();
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
      {#if group}
        <GroupIdentity
          {group}
          {groupSize}
          onOpenGroup={() => (showGroupModal = true)}
        />
      {:else}
        <a class="row-link" href={`#/images/${image.id}${detailsQuery}`}
          >{image.name}</a
        >
        <span class="muted source">{imageSource(image)}</span>
        <UpstreamLink registry={image.registry} repository={image.repository} />
      {/if}
    </div>
    <span class="versions muted">
      <span>Current <span class="tag-value">{image.tag}</span></span>
      {#if flagged && suggestedTag}
        <span>Suggested <strong>{suggestedTag}</strong></span>
      {:else if blocked && group?.partialTag}
        <span>Newest <strong>{group.partialTag.tag}</strong></span>
      {/if}
    </span>
  </header>

  {#if group}
    <GroupMembers {group} {blocked} {detailsQuery} />
  {/if}

  {#if lastChange}
    <TagChangeNotice
      from={lastChange.from}
      to={lastChange.to}
      groupSize={group ? groupSize : 0}
      disabled={working || locked}
      onRevert={revert}
    />
  {/if}

  {#if pinned}
    <p class="notice">
      <Pin size={14} strokeWidth={2} />
      <span class="notice-text">
        {group ? "Group pinned" : "Pinned"} to <strong>{image.tag}</strong>. New
        versions won't be flagged.
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
  {:else if blocked}
    <GroupBlockedNotice
      {group}
      {groupSize}
      disabled={working || locked}
      onManage={() => (showGroupModal = true)}
    />

    <div class="row-actions">
      {#if locked}
        <RefreshingIndicator text="Checking upstream…" />
      {/if}
      <Button
        variant="secondary"
        size="sm"
        disabled={working || locked}
        onclick={() => setPinned(true)}
      >
        <Pin size={14} strokeWidth={2} />
        Pin group
      </Button>
    </div>
  {:else if flagged}
    {#if group?.partialTag}
      <p class="partial-note muted">
        <span class="tag-name">{group.partialTag.tag}</span> is newer, but
        {group.partialTag.missing.join(", ")}
        {group.partialTag.missing.length === 1 ? "doesn't" : "don't"} have it yet.
      </p>
    {/if}

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
        showFirstSeen
      />
      {#if group}
        <p class="group-hint muted">
          Only tags available on {allImages(groupSize)} are shown.
        </p>
      {/if}
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
        <Tags size={14} strokeWidth={2} />
        All tags…
      </Button>
      <Button
        variant="secondary"
        size="sm"
        disabled={working || locked}
        onclick={() => setPinned(true)}
      >
        <Pin size={14} strokeWidth={2} />
        {group ? "Pin group" : "Pin"}
      </Button>
      <Button
        size="sm"
        disabled={!hasNewSelection || working || locked}
        onclick={applySelectedTag}
      >
        <ArrowUp size={14} strokeWidth={2.5} />
        {#if !hasNewSelection}
          Select a new tag
        {:else if group}
          Update {allCount(groupSize)} to {selectedTag}
        {:else}
          Update to {selectedTag}
        {/if}
      </Button>
    </div>
  {:else if !lastChange}
    <p class="muted empty-note">
      No update is flagged for this {group ? "group" : "image"} anymore.
    </p>
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
    groupName={group?.name}
    {groupSize}
    onClose={() => (showTagModal = false)}
    onTagUpdated={(tag) => settleTagChange(image.tag, tag)}
    onTagsRefreshed={reloadRow}
  />
{/if}

{#if group}
  <ImageGroupModal
    open={showGroupModal}
    {group}
    onClose={() => (showGroupModal = false)}
    onSaved={handleGroupSaved}
    onDeleted={handleGroupDeleted}
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
    font-weight: 500;
  }

  .tag-name {
    font-family: monospace;
  }

  .partial-note,
  .group-hint {
    margin: 0;
    font-size: 0.8125rem;
  }

  .group-hint {
    margin-top: var(--space-2);
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
