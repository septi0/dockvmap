<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import ArrowUpRight from "@lucide/svelte/icons/arrow-up-right";
  import CircleAlert from "@lucide/svelte/icons/circle-alert";
  import Layers from "@lucide/svelte/icons/layers";
  import Pin from "@lucide/svelte/icons/pin";
  import Plus from "@lucide/svelte/icons/plus";
  import Sparkles from "@lucide/svelte/icons/sparkles";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import X from "@lucide/svelte/icons/x";
  import AppShell from "../lib/components/AppShell.svelte";
  import AsyncState from "../lib/components/AsyncState.svelte";
  import Button from "../lib/components/Button.svelte";
  import CopyButton from "../lib/components/CopyButton.svelte";
  import GroupPullNotice from "../lib/components/GroupPullNotice.svelte";
  import FilterBar from "../lib/components/FilterBar.svelte";
  import ImageGroupModal from "../lib/components/ImageGroupModal.svelte";
  import PageTitle from "../lib/components/PageTitle.svelte";
  import Pagination from "../lib/components/Pagination.svelte";
  import SearchInput from "../lib/components/SearchInput.svelte";
  import { listImageGroups } from "../lib/api/imageGroups";
  import { getPullInfo } from "../lib/api/metrics";
  import type { PullInfo } from "../lib/api/types/metrics";
  import { imageReference } from "../lib/utils/pullInfo";
  import { IMAGES_MAX_LIMIT, IMAGES_PAGE_SIZE, listImages } from "../lib/api/images";
  import type { Image } from "../lib/api/types/images";
  import type { ImageGroup } from "../lib/api/types/imageGroups";
  import { tagRefreshStatus } from "../lib/services/tagRefreshStatus";
  import { toast } from "../lib/services/toast";
  import { updatesCount } from "../lib/services/updatesCount";
  import {
    suggestImageGroups,
    type ImageGroupSuggestion,
  } from "../lib/utils/imageGroupSuggestions";
  import { createListView } from "../lib/utils/listView.svelte";

  const DISMISSED_KEY = "dockvmap-dismissed-group-suggestions";
  const VISIBLE_MEMBERS = 4;

  const STATUS_OPTIONS = [
    { value: "", label: "All groups" },
    { value: "updateAvailable", label: "Updates available" },
    { value: "pinned", label: "Pinned" },
  ];

  const view = createListView<
    { search: string; status: string; offset: number },
    ImageGroup
  >({
    routePath: "/groups",
    defaults: { search: "", status: "", offset: 0 },
    errorFallback: "Failed to load groups",
    fetch: (q) =>
      listImageGroups({
        offset: q.offset,
        limit: IMAGES_PAGE_SIZE,
        search: q.search || undefined,
        status: (q.status || undefined) as "updateAvailable" | "pinned" | undefined,
      }).then((r) => ({ items: r.groups, total: r.total })),
  });

  let ungroupedImages = $state<Image[]>([]);
  let pullInfo = $state<PullInfo | null>(null);
  let dismissed = $state<string[]>(readDismissed());

  let modalOpen = $state(false);
  let modalGroup = $state<ImageGroup | null>(null);
  let modalName = $state("");
  let modalImages = $state<Image[]>([]);

  let suggestions = $derived(
    suggestImageGroups(ungroupedImages).filter(
      (suggestion) => !dismissed.includes(suggestion.key),
    ),
  );

  function readDismissed(): string[] {
    try {
      const stored = localStorage.getItem(DISMISSED_KEY);
      return stored ? JSON.parse(stored) : [];
    } catch {
      return [];
    }
  }

  function dismissSuggestion(suggestion: ImageGroupSuggestion) {
    dismissed = [...dismissed, suggestion.key];

    try {
      localStorage.setItem(DISMISSED_KEY, JSON.stringify(dismissed));
    } catch {}
  }

  async function loadSuggestions() {
    try {
      const result = await listImages({
        offset: 0,
        limit: IMAGES_MAX_LIMIT,
        ungrouped: true,
      });
      ungroupedImages = result.images;
    } catch {
      ungroupedImages = [];
    }
  }

  onMount(view.init);
  onMount(loadSuggestions);
  onMount(() => {
    getPullInfo()
      .then((info) => (pullInfo = info))
      .catch(() => {});
  });
  onMount(() => tagRefreshStatus.watch());
  onMount(() =>
    tagRefreshStatus.onCompleted(() => {
      if (view.loaded) view.reload();
    }),
  );

  function openGroup(group: ImageGroup) {
    modalGroup = group;
    modalName = "";
    modalImages = [];
    modalOpen = true;
  }

  function openNew(name = "", images: Image[] = []) {
    modalGroup = null;
    modalName = name;
    modalImages = images;
    modalOpen = true;
  }

  function handleRowClick(event: MouseEvent, group: ImageGroup) {
    if ((event.target as HTMLElement).closest("a, button")) return;
    openGroup(group);
  }

  function refreshAfterChange() {
    view.reload();
    loadSuggestions();
    updatesCount.refresh();
  }

  function handleSaved(name: string) {
    toast.success(`Group "${name}" saved.`);
    refreshAfterChange();
  }

  function handleDeleted(name: string) {
    toast.success(`Group "${name}" deleted.`);
    refreshAfterChange();
  }

  function failedChecks(group: ImageGroup) {
    return group.images.filter((image) => image.lastCheckError).length;
  }

  function imagesHref(group: ImageGroup) {
    return `/images?group=${group.id}`;
  }

  function updatesHref(group: ImageGroup) {
    return `/updates?search=${encodeURIComponent(group.name)}`;
  }
</script>

<PageTitle title="Groups" />

<AppShell>
  <div class="list-header">
    <div>
      <div class="title-row">
        <Layers size={20} strokeWidth={1.75} />
        <h1>Groups</h1>
      </div>
      <p class="subtitle">Images that always move to the same version together.</p>
    </div>
    <Button onclick={() => openNew()}>
      <Plus size={16} strokeWidth={2} />
      New group
    </Button>
  </div>

  {#if suggestions.length > 0 && !view.hasActiveFilters}
    <section class="card suggestions" aria-label="Suggested groups">
      <h2>
        <Sparkles size={16} strokeWidth={2} />
        Suggested groups
      </h2>
      <p class="muted hint">
        These images come from the same place and are on the same tag.
      </p>
      <ul class="suggestion-list">
        {#each suggestions as suggestion (suggestion.key)}
          <li class="suggestion">
            <div class="suggestion-main">
              <span class="suggestion-name">{suggestion.name}</span>
              <span class="muted suggestion-images">
                {suggestion.images.map((image) => image.name).join(", ")} on
                <span class="tag-value">{suggestion.tag}</span>
              </span>
            </div>
            <Button
              variant="secondary"
              size="sm"
              onclick={() => openNew(suggestion.name, suggestion.images)}
            >
              <Plus size={14} strokeWidth={2} />
              Create group
            </Button>
            <button
              type="button"
              class="icon-button"
              aria-label={`Dismiss suggestion ${suggestion.name}`}
              onclick={() => dismissSuggestion(suggestion)}
            >
              <X size={16} strokeWidth={1.75} />
            </button>
          </li>
        {/each}
      </ul>
    </section>
  {/if}

  <FilterBar active={view.hasActiveFilters} onClear={view.clear}>
    <SearchInput
      bind:value={view.filters.search}
      placeholder="Search groups and images…"
      onSearch={view.applyFilters}
    />
    <div class="filter-field">
      <span class="filter-label">Status</span>
      <select
        class="input filter-control"
        class:is-active={view.filters.status !== ""}
        bind:value={view.filters.status}
        onchange={view.applyFilters}
      >
        {#each STATUS_OPTIONS as option (option.value)}
          <option value={option.value}>{option.label}</option>
        {/each}
      </select>
    </div>
  </FilterBar>

  <AsyncState
    loading={view.loading && !view.loaded}
    busy={view.loading && view.loaded}
    error={view.error}
    empty={view.items.length === 0}
    emptyMessage={view.hasActiveFilters
      ? "No groups match your filters."
      : "No groups yet. Group images that release together, and updating one updates them all."}
    columns={5}
  >
    {#snippet emptyAction()}
      {#if !view.hasActiveFilters}
        <Button onclick={() => openNew()}>
          <Plus size={16} strokeWidth={2} />
          New group
        </Button>
      {/if}
    {/snippet}

    <div class="card table-wrap">
      <table class="table">
        <thead>
          <tr>
            <th>Name</th>
            <th class="phone-hidden">Images</th>
            <th class="phone-hidden">Tag</th>
            <th>Status</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each view.items as group (group.id)}
            {@const failed = failedChecks(group)}
            {@const hidden = group.images.length - VISIBLE_MEMBERS}
            <tr class="clickable" onclick={(event) => handleRowClick(event, group)}>
              <td>
                <button
                  type="button"
                  class="group-name"
                  onclick={() => openGroup(group)}>{group.name}</button
                >
              </td>
              <td class="members phone-hidden">
                <a class="row-link" href={imagesHref(group)} use:link>
                  {group.images
                    .slice(0, VISIBLE_MEMBERS)
                    .map((image) => image.name)
                    .join(", ")}{hidden > 0 ? ` +${hidden} more` : ""}
                </a>
              </td>
              <td class="phone-hidden">
                <span class="tag-value">{group.tag}</span>
              </td>
              <td>
                <span class="status">
                  {#if group.pinned}
                    <span class="badge badge-success">
                      <Pin size={12} strokeWidth={2} />
                      Pinned
                    </span>
                  {:else if group.updateAvailableTag}
                    <span class="badge badge-warning">
                      <TriangleAlert size={12} strokeWidth={2} />
                      Update to {group.updateAvailableTag}
                    </span>
                  {:else if group.updateAvailable}
                    <span
                      class="badge badge-warning"
                      title={group.partialTag
                        ? `${group.partialTag.tag} is missing from ${group.partialTag.missing.join(", ")}`
                        : undefined}
                    >
                      <TriangleAlert size={12} strokeWidth={2} />
                      Update blocked
                    </span>
                  {:else}
                    <span class="muted">Up to date</span>
                  {/if}
                  {#if failed > 0}
                    <span
                      class="check-error-icon"
                      title={`${failed} ${failed === 1 ? "image" : "images"} failed the last tag check`}
                      aria-label={`${failed} failed tag checks`}
                    >
                      <CircleAlert size={14} strokeWidth={2} />
                    </span>
                  {/if}
                </span>
                <GroupPullNotice {group} />
              </td>
              <td class="actions">
                <span class="row-tools">
                  {#if pullInfo}
                    {@const info = pullInfo}
                    <span class="row-reveal"><CopyButton
                      text={group.images.map((image) => imageReference(info, image.name)).join("\n")}
                      label={`Copy image references for ${group.name}`}
                      copiedMessage="Image references copied to clipboard."
                    /></span>
                  {/if}
                  {#if group.updateAvailable && !group.pinned}
                    <a class="review-link" href={updatesHref(group)} use:link>
                      Review
                      <ArrowUpRight size={14} strokeWidth={2} />
                    </a>
                  {/if}
                </span>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <Pagination
      total={view.total}
      limit={IMAGES_PAGE_SIZE}
      offset={view.filters.offset}
      onOffsetChange={view.setOffset}
    />
  </AsyncState>
</AppShell>

{#if modalOpen}
  <ImageGroupModal
    open
    group={modalGroup}
    initialName={modalName}
    initialImages={modalImages}
    onClose={() => (modalOpen = false)}
    onSaved={handleSaved}
    onDeleted={handleDeleted}
  />
{/if}

<style>
  .suggestions {
    padding: var(--space-4);
    margin-bottom: var(--space-4);
  }

  .suggestions h2 {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin: 0 0 var(--space-1);
    font-size: 0.9375rem;
  }

  .hint {
    margin: 0 0 var(--space-3);
    font-size: 0.8125rem;
  }

  .suggestion-list {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .suggestion {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) 0;
    border-top: 1px solid var(--color-border);
  }

  .suggestion-main {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-width: 0;
  }

  .suggestion-name {
    font-size: 0.875rem;
    font-weight: 500;
    overflow-wrap: anywhere;
  }

  .suggestion-images {
    font-size: 0.8125rem;
    overflow-wrap: anywhere;
  }

  .group-name {
    padding: 0;
    overflow-wrap: break-word;
    border: none;
    background: none;
    color: inherit;
    font: inherit;
    font-weight: 500;
    text-align: left;
    cursor: pointer;
  }

  .group-name:hover {
    text-decoration: underline;
  }

  .members {
    max-width: 360px;
  }

  .row-link {
    color: var(--color-text-muted);
  }

  .status {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
  }

  .actions {
    text-align: right;
    white-space: nowrap;
  }

  .row-tools {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
  }

  .review-link {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    color: var(--color-accent);
    font-size: 0.8125rem;
    font-weight: 500;
    text-decoration: none;
  }

  .review-link:hover {
    text-decoration: underline;
  }
</style>
