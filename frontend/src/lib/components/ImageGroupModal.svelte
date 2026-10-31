<script lang="ts">
  import { untrack } from "svelte";
  import Pin from "@lucide/svelte/icons/pin";
  import Plus from "@lucide/svelte/icons/plus";
  import Trash2 from "@lucide/svelte/icons/trash-2";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import X from "@lucide/svelte/icons/x";
  import AsyncState from "./AsyncState.svelte";
  import Button from "./Button.svelte";
  import ConfirmDialog from "./ConfirmDialog.svelte";
  import Field from "./Field.svelte";
  import Modal from "./Modal.svelte";
  import Select from "./Select.svelte";
  import TagFamilyPicker from "./TagFamilyPicker.svelte";
  import {
    createImageGroup,
    deleteImageGroup,
    getCommonImageTags,
    IMAGE_GROUPS_MAX_LIMIT,
    listImageGroups,
    updateImageGroup,
  } from "../api/imageGroups";
  import { listImages } from "../api/images";
  import { errorMessage } from "../api/client";
  import { debounce } from "../utils/debounce";
  import { allImages } from "../utils/format";
  import type { Image, TagGroup } from "../api/types/images";
  import type { ImageGroup } from "../api/types/imageGroups";

  const SEARCH_LIMIT = 6;
  const SEARCH_DELAY_MS = 300;
  const MIN_IMAGES = 2;

  let {
    open,
    group = null,
    image = null,
    initialName = "",
    initialImages = [],
    onClose,
    onSaved,
    onDeleted,
  }: {
    open: boolean;
    group?: ImageGroup | null;
    image?: Image | null;
    initialName?: string;
    initialImages?: Image[];
    onClose: () => void;
    onSaved: (name: string) => void;
    onDeleted?: (name: string) => void;
  } = $props();

  let name = $state("");
  let images = $state<Image[]>([]);
  let originalIds = $state<number[]>([]);
  let groupId = $state<number | null>(null);
  let existingGroups = $state<ImageGroup[]>([]);
  let joinTarget = $state("");

  let tagGroups = $state<TagGroup[]>([]);
  let selectedTag = $state<string | null>(null);
  let tagsLoading = $state(true);
  let tagsError = $state<string | null>(null);
  let tagsToken = 0;

  let search = $state("");
  let results = $state<Image[]>([]);
  let searchError = $state<string | null>(null);
  let searchToken = 0;

  let saving = $state(false);
  let saveError = $state<string | null>(null);

  let showDeleteConfirm = $state(false);
  let deleting = $state(false);
  let deleteError = $state<string | null>(null);

  let modalTitle = $derived(
    group ? "Manage group" : image ? "Add to group" : "New group",
  );
  let imageIds = $derived(images.map((item) => item.id));
  let joinOptions = $derived([
    { value: "", label: "New group" },
    ...existingGroups.map((item) => ({
      value: String(item.id),
      label: item.name,
    })),
  ]);
  let enoughImages = $derived(images.length >= MIN_IMAGES);
  let noCommonTags = $derived(
    !tagsLoading && !tagsError && tagGroups.length === 0,
  );
  let moving = $derived(
    selectedTag ? images.filter((item) => item.tag !== selectedTag) : [],
  );
  let pinnedImages = $derived(images.filter((item) => item.pinned));
  let pinsWholeGroup = $derived(
    pinnedImages.length > 0 && pinnedImages.length < images.length,
  );
  let removedCount = $derived(
    originalIds.filter((id) => !imageIds.includes(id)).length,
  );
  let canSave = $derived(
    !saving && name.trim() !== "" && enoughImages && !!selectedTag,
  );
  let showCandidates = $derived(search.trim() !== "" || !enoughImages);
  let candidates = $derived(
    results.filter((item) => !imageIds.includes(item.id)),
  );

  const debouncedSearch = debounce(loadResults, SEARCH_DELAY_MS);

  function hasTag(tag: string | null) {
    return tagGroups.some((family) =>
      family.tags.some((item) => item.tag === tag),
    );
  }

  function mostUsedCommonTag(): string | null {
    const counts = new Map<string, number>();

    for (const item of images) {
      if (hasTag(item.tag)) {
        counts.set(item.tag, (counts.get(item.tag) ?? 0) + 1);
      }
    }

    let best: string | null = null;

    for (const [tag, count] of counts) {
      if (best === null || count > (counts.get(best) ?? 0)) best = tag;
    }

    return best;
  }

  async function loadTags() {
    const requestId = ++tagsToken;
    tagsLoading = true;
    tagsError = null;

    if (images.length === 0) {
      tagGroups = [];
      selectedTag = null;
      tagsLoading = false;
      return;
    }

    try {
      const result = await getCommonImageTags(imageIds);
      if (requestId !== tagsToken) return;

      tagGroups = result.tagGroups;

      if (!hasTag(selectedTag)) selectedTag = mostUsedCommonTag();
    } catch (err) {
      if (requestId !== tagsToken) return;
      tagsError = errorMessage(err, "Failed to load common tags");
    } finally {
      if (requestId === tagsToken) tagsLoading = false;
    }
  }

  async function loadResults(term: string) {
    const requestId = ++searchToken;
    searchError = null;

    try {
      const result = await listImages({
        offset: 0,
        limit: SEARCH_LIMIT + images.length,
        search: term || undefined,
      });
      if (requestId !== searchToken) return;

      results = result.images;
    } catch (err) {
      if (requestId !== searchToken) return;
      searchError = errorMessage(err, "Failed to search images");
    }
  }

  async function loadExistingGroups() {
    try {
      const result = await listImageGroups({ limit: IMAGE_GROUPS_MAX_LIMIT });
      existingGroups = result.groups;
    } catch {
      existingGroups = [];
    }
  }

  function startFrom(source: ImageGroup | null) {
    groupId = source?.id ?? null;
    name = source?.name ?? initialName;
    originalIds = source?.images.map((item) => item.id) ?? [];
    selectedTag = source?.tag ?? null;

    const members = source ? [...source.images] : [...initialImages];

    if (image && !members.some((item) => item.id === image.id)) {
      members.push(image);
    }

    images = members;
    saveError = null;
    loadTags();
  }

  $effect(() => {
    if (!open) return;

    untrack(() => {
      search = "";
      joinTarget = "";
      results = [];
      startFrom(group);
      loadResults("");

      if (!group && image) loadExistingGroups();
    });
  });

  function handleJoinChange() {
    const target = existingGroups.find(
      (item) => String(item.id) === joinTarget,
    );

    startFrom(target ?? null);
  }

  function addImage(item: Image) {
    images = [...images, item];
    loadTags();
  }

  function removeImage(item: Image) {
    images = images.filter((candidate) => candidate.id !== item.id);
    loadTags();
  }

  async function save() {
    if (!canSave || !selectedTag) return;

    const params = { name: name.trim(), imageIds, tag: selectedTag };

    saveError = null;
    saving = true;

    try {
      if (groupId) await updateImageGroup(groupId, params);
      else await createImageGroup(params);

      onSaved(params.name);
      onClose();
    } catch (err) {
      saveError = errorMessage(err, "Failed to save group");
    } finally {
      saving = false;
    }
  }

  async function confirmDelete() {
    if (!groupId) return;

    deleteError = null;
    deleting = true;

    try {
      await deleteImageGroup(groupId);
      showDeleteConfirm = false;
      onDeleted?.(name);
      onClose();
    } catch (err) {
      deleteError = errorMessage(err, "Failed to delete group");
    } finally {
      deleting = false;
    }
  }

  function cancelDelete() {
    showDeleteConfirm = false;
    deleteError = null;
  }
</script>

<Modal {open} {onClose} title={modalTitle} size="lg">
  <div class="group-form">
    {#if !group && image && existingGroups.length > 0}
      <Select
        label="Group"
        bind:value={joinTarget}
        options={joinOptions}
        onchange={handleJoinChange}
      />
    {/if}

    <Field label="Group name" bind:value={name} required autofocus={!group} />

    <div class="field">
      <span class="field-label">Images</span>
      {#if images.length === 0}
        <p class="muted empty">Add at least {MIN_IMAGES} images.</p>
      {:else}
        <ul class="members">
          {#each images as item (item.id)}
            {@const moves = !!selectedTag && item.tag !== selectedTag}
            <li class="member">
              <span class="member-name">{item.name}</span>
              <span class="member-tag" class:moves>
                {item.tag}{#if moves}
                  &nbsp;&rarr; {selectedTag}{/if}
              </span>
              <button
                type="button"
                class="icon-button"
                aria-label={`Remove ${item.name} from group`}
                onclick={() => removeImage(item)}
              >
                <X size={16} strokeWidth={1.75} />
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>

    <div class="field">
      <span class="field-label">Add images</span>
      <input
        type="search"
        class="input"
        aria-label="Search images to add"
        placeholder="Search images…"
        bind:value={search}
        oninput={() => debouncedSearch(search)}
      />
      {#if searchError}
        <p class="error">
          <TriangleAlert size={16} strokeWidth={2} />
          {searchError}
        </p>
      {:else if showCandidates && candidates.length === 0}
        <p class="muted empty">No more images found.</p>
      {:else if showCandidates}
        <ul class="candidates">
          {#each candidates.slice(0, SEARCH_LIMIT) as item (item.id)}
            {@const taken = !!item.group && item.group.id !== groupId}
            <li>
              <button
                type="button"
                class="candidate"
                disabled={taken}
                onclick={() => addImage(item)}
              >
                <Plus size={14} strokeWidth={2} />
                <span class="member-name">{item.name}</span>
                {#if taken}
                  <span class="muted">In {item.group?.name}</span>
                {:else}
                  <span class="member-tag">{item.tag}</span>
                {/if}
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>

    <div class="field">
      <span class="field-label">Shared tag</span>
      <AsyncState
        loading={tagsLoading}
        error={tagsError}
        empty={noCommonTags}
        emptyVariant="text"
        emptyMessage={images.length === 0
          ? "Add images to see their common tags."
          : "These images have no tag in common, so they can't share a version."}
      >
        <div class="tags-scroll">
          <TagFamilyPicker
            {tagGroups}
            bind:selectedTag
            currentTag={group?.tag ?? null}
            searchable
          />
        </div>
        <p class="muted hint">
          {enoughImages
            ? `Only tags available on ${allImages(images.length)} are shown.`
            : `A group needs at least ${MIN_IMAGES} images.`}
        </p>
      </AsyncState>
    </div>

    {#if selectedTag && moving.length > 0}
      <p class="warning-box">
        <TriangleAlert size={16} strokeWidth={2} />
        <span>
          {moving.length === 1
            ? `${moving[0].name} moves`
            : `${moving.length} images move`}
          to <strong>{selectedTag}</strong> when you save.
        </span>
      </p>
    {/if}

    {#if pinsWholeGroup}
      <p class="warning-box">
        <Pin size={16} strokeWidth={2} />
        <span>
          {pinnedImages.map((item) => item.name).join(", ")}
          {pinnedImages.length === 1 ? "is" : "are"} pinned, so the whole group
          will be pinned.
        </span>
      </p>
    {/if}

    {#if removedCount > 0}
      <p class="muted hint">Removed images stay on their current tag.</p>
    {/if}

    {#if saveError}
      <p class="error">
        <TriangleAlert size={16} strokeWidth={2} />
        {saveError}
      </p>
    {/if}

    <div class="modal-actions split">
      {#if groupId}
        <Button variant="danger" onclick={() => (showDeleteConfirm = true)}>
          <Trash2 size={16} strokeWidth={1.75} />
          Delete group
        </Button>
      {:else}
        <span></span>
      {/if}
      <div class="modal-actions-right">
        <Button variant="secondary" onclick={onClose}>Cancel</Button>
        <Button disabled={!canSave} onclick={save}>
          {saving ? "Saving…" : "Save group"}
        </Button>
      </div>
    </div>
  </div>
</Modal>

<ConfirmDialog
  open={showDeleteConfirm}
  title="Delete group"
  message={`Delete "${name}"? Its images stay as they are, on their current tag.`}
  confirmLabel="Delete group"
  danger
  error={deleteError}
  submitting={deleting}
  onConfirm={confirmDelete}
  onCancel={cancelDelete}
/>

<style>
  .group-form {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .empty,
  .hint {
    margin: 0;
    font-size: 0.8125rem;
  }

  .hint {
    margin-top: var(--space-2);
  }

  .members,
  .candidates {
    list-style: none;
    margin: 0;
    padding: 0;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
  }

  .candidates {
    margin-top: var(--space-2);
  }

  .member,
  .candidate {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-3);
    font-size: 0.875rem;
  }

  .member {
    border-bottom: 1px solid var(--color-border);
  }

  .member:last-child,
  .candidates li:last-child .candidate {
    border-bottom: none;
  }

  .candidate {
    width: 100%;
    border: none;
    border-bottom: 1px solid var(--color-border);
    background: none;
    color: inherit;
    font: inherit;
    font-size: 0.875rem;
    text-align: left;
    cursor: pointer;
  }

  .candidate:hover:not(:disabled) {
    background: var(--color-surface-hover);
  }

  .candidate:disabled {
    cursor: not-allowed;
    opacity: 0.6;
  }

  .member-name {
    flex: 1;
    min-width: 0;
    font-weight: 500;
    overflow-wrap: anywhere;
  }

  .member-tag {
    flex-shrink: 0;
    max-width: 60%;
    font-family: monospace;
    font-size: 0.8125rem;
    color: var(--color-text-muted);
    overflow-wrap: anywhere;
  }

  .member-tag.moves {
    color: var(--color-warning);
  }

  .tags-scroll {
    max-height: 200px;
    overflow-y: auto;
    overscroll-behavior: contain;
    padding: 2px;
  }

  .modal-actions {
    padding-top: var(--space-4);
    border-top: 1px solid var(--color-border);
  }

  .modal-actions-right {
    flex-shrink: 0;
  }
</style>
