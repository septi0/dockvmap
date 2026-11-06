<script lang="ts">
  import { onMount } from "svelte";
  import { push } from "svelte-spa-router";
  import Plus from "@lucide/svelte/icons/plus";
  import Package from "@lucide/svelte/icons/package";
  import RefreshCw from "@lucide/svelte/icons/refresh-cw";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import CircleAlert from "@lucide/svelte/icons/circle-alert";
  import Pin from "@lucide/svelte/icons/pin";
  import Layers from "@lucide/svelte/icons/layers";
  import AppShell from "../lib/components/AppShell.svelte";
  import PageTitle from "../lib/components/PageTitle.svelte";
  import AsyncState from "../lib/components/AsyncState.svelte";
  import Pagination from "../lib/components/Pagination.svelte";
  import FilterBar from "../lib/components/FilterBar.svelte";
  import SearchInput from "../lib/components/SearchInput.svelte";
  import Button from "../lib/components/Button.svelte";
  import ConfirmDialog from "../lib/components/ConfirmDialog.svelte";
  import RefreshingIndicator from "../lib/components/RefreshingIndicator.svelte";
  import ImageGroupModal from "../lib/components/ImageGroupModal.svelte";
  import { listImages, IMAGES_PAGE_SIZE } from "../lib/api/images";
  import CopyButton from "../lib/components/CopyButton.svelte";
  import StateIndicator from "../lib/components/StateIndicator.svelte";
  import { getPullInfo } from "../lib/api/metrics";
  import { listImageRollouts } from "../lib/api/hosts";
  import type { PullInfo } from "../lib/api/types/metrics";
  import type { ImageRollout } from "../lib/api/types/hosts";
  import { pullCommand } from "../lib/utils/pullInfo";
  import { getImageGroup, IMAGE_GROUPS_MAX_LIMIT, listImageGroups } from "../lib/api/imageGroups";
  import { listRegistries } from "../lib/api/registries";
  import type { Registry } from "../lib/api/types/registries";
  import { triggerTagRefresh } from "../lib/api/worker";
  import { toast } from "../lib/services/toast";
  import { tagRefreshStatus } from "../lib/services/tagRefreshStatus";
  import { updatesCount } from "../lib/services/updatesCount";
  import { errorMessage } from "../lib/api/client";
  import {
    IMAGE_STATUS_FILTERS,
    type Image,
    type ImageSortField,
    type ImageStatusFilter,
  } from "../lib/api/types/images";
  import type { ImageGroup } from "../lib/api/types/imageGroups";
  import { formatCount, formatDate, formatRelativeTime, imageParts } from "../lib/utils/format";
  import { writeListQuery } from "../lib/utils/listQuery";
  import { imageSource, registryLabel } from "../lib/utils/registries";
  import { createVisibilityPoller } from "../lib/utils/visibilityPoller";
  import {
    createListView,
    NO_SORT,
    sortParams,
  } from "../lib/utils/listView.svelte";
  import SortableHeader from "../lib/components/SortableHeader.svelte";
  import RolloutSummary from "../lib/components/hosts/RolloutSummary.svelte";

  const STATUS_OPTIONS: { value: ImageStatusFilter | ""; label: string }[] = [
    { value: "", label: "All images" },
    { value: "updateAvailable", label: "Updates available" },
    { value: "failedCheck", label: "Failed checks" },
    { value: "pinned", label: "Pinned" },
  ];

  let showRefreshAllConfirm = $state(false);
  let refreshingAll = $state(false);
  let refreshAllError = $state<string | null>(null);

  let tagRefreshRunning = $derived($tagRefreshStatus.data?.running ?? false);

  const NOT_GROUPED = "none";
  const ROLLOUT_REFRESH_INTERVAL_MS = 15_000;

  let groups = $state<ImageGroup[]>([]);
  let pullInfo = $state<PullInfo | null>(null);
  let rollouts = $state(new Map<number, ImageRollout>());
  let managedGroup = $state<ImageGroup | null>(null);
  let usedRegistries = $state<Registry[]>([]);
  let groupError = $state<string | null>(null);

  const view = createListView<
    {
      search: string;
      status: string;
      group: string;
      registry: string;
      sort: string;
      order: string;
      offset: number;
    },
    Image
  >({
    routePath: "/images",
    defaults: { search: "", status: "", group: "", registry: "", ...NO_SORT, offset: 0 },
    errorFallback: "Failed to load images",
    sortFields: {
      name: "asc",
      lastPulled: "desc",
      status: "asc",
    },
    fetch: (q) =>
      listImages({
        ...sortParams<ImageSortField>(q),
        offset: q.offset,
        limit: IMAGES_PAGE_SIZE,
        search: q.search || undefined,
        status: (IMAGE_STATUS_FILTERS as readonly string[]).includes(q.status)
          ? (q.status as ImageStatusFilter)
          : undefined,
        ungrouped: q.group === NOT_GROUPED ? true : undefined,
        groupId:
          q.group && q.group !== NOT_GROUPED ? Number(q.group) : undefined,
        registryId: q.registry ? Number(q.registry) : undefined,
      }).then((r) => ({ items: r.images, total: r.total })),
  });

  async function loadGroups() {
    try {
      const result = await listImageGroups({ limit: IMAGE_GROUPS_MAX_LIMIT });
      groups = result.groups;
    } catch {
      groups = [];
    }
  }

  async function loadRegistries() {
    try {
      usedRegistries = (await listRegistries()).filter((registry) => registry.imageCount > 0);
    } catch {
      usedRegistries = [];
    }
  }

  async function loadRollouts() {
    try {
      const result = await listImageRollouts();
      rollouts = new Map(result.map((r) => [r.imageId, r]));
    } catch {}
  }

  async function openGroup(id: number) {
    groupError = null;

    try {
      managedGroup = await getImageGroup(id);
    } catch (err) {
      groupError = errorMessage(err, "Failed to load group");
    }
  }

  function handleGroupChanged(message: string) {
    toast.success(message);
    view.reload();
    loadGroups();
    updatesCount.refresh();
  }

  onMount(view.init);
  onMount(loadGroups);
  onMount(loadRegistries);
  onMount(() =>
    createVisibilityPoller({
      tick: loadRollouts,
      intervalMs: ROLLOUT_REFRESH_INTERVAL_MS,
    }).start(),
  );
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

  function rowQuery() {
    return writeListQuery({ ...view.filters });
  }

  function openImage(event: MouseEvent, id: number) {
    if ((event.target as HTMLElement).closest("a, button")) return;
    push(`/images/${id}${rowQuery()}`);
  }

  function openRefreshAll() {
    if (tagRefreshRunning) return;
    refreshAllError = null;
    showRefreshAllConfirm = true;
  }

  function cancelRefreshAll() {
    showRefreshAllConfirm = false;
    refreshAllError = null;
  }

  async function confirmRefreshAll() {
    refreshAllError = null;
    refreshingAll = true;

    try {
      await triggerTagRefresh();
      showRefreshAllConfirm = false;
      tagRefreshStatus.notifyTriggered();
      toast.success("Tag check started - running in the background.");
    } catch (err) {
      refreshAllError = errorMessage(err, "Failed to start tag check");
      tagRefreshStatus.refresh();
    } finally {
      refreshingAll = false;
    }
  }
</script>

{#snippet rolloutInfo(rollout: ImageRollout)}
  {#if rollout.total > 0}
    <RolloutSummary {rollout} />
  {/if}
  {#if rollout.hostsOffline > 0}
    <div class="source muted" title="These hosts ran this image when they were last online">
      {formatCount(rollout.hostsOffline, "host")} offline
    </div>
  {/if}
  {#if rollout.hostsDockerError > 0}
    <div class="source muted" title="These hosts ran this image when their agent could last read Docker">
      {formatCount(rollout.hostsDockerError, "host")} can't read Docker
    </div>
  {/if}
{/snippet}

<PageTitle title="Virtual Images" />

<AppShell>
  <div class="list-header">
    <div class="title-row">
      <Package size={20} strokeWidth={1.75} />
      <h1>Virtual Images</h1>
    </div>
    <div class="header-actions">
      {#if tagRefreshRunning}
        <RefreshingIndicator text="Checking all images for tag changes…" />
      {/if}
      <Button
        variant="secondary"
        onclick={openRefreshAll}
        disabled={tagRefreshRunning || refreshingAll}
      >
        <RefreshCw size={16} strokeWidth={2} />
        Refresh all tags
      </Button>
      <Button onclick={() => push("/images/new")}>
        <Plus size={16} strokeWidth={2} />
        Add virtual image
      </Button>
    </div>
  </div>

  <FilterBar active={view.hasActiveFilters} onClear={view.clear}>
    <SearchInput
      bind:value={view.filters.search}
      placeholder="Search images…"
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
    {#if groups.length > 0}
      <div class="filter-field">
        <span class="filter-label">Group</span>
        <select
          class="input filter-control"
          class:is-active={view.filters.group !== ""}
          bind:value={view.filters.group}
          onchange={view.applyFilters}
        >
          <option value="">All images</option>
          <option value={NOT_GROUPED}>Not in a group</option>
          {#each groups as group (group.id)}
            <option value={String(group.id)}>{group.name}</option>
          {/each}
        </select>
      </div>
    {/if}
    {#if usedRegistries.length > 0}
      <div class="filter-field">
        <span class="filter-label">Registry</span>
        <select
          class="input filter-control"
          class:is-active={view.filters.registry !== ""}
          bind:value={view.filters.registry}
          onchange={view.applyFilters}
        >
          <option value="">All registries</option>
          {#each usedRegistries as registry (registry.id)}
            <option value={String(registry.id)}>{registryLabel(registry)}</option>
          {/each}
        </select>
      </div>
    {/if}
  </FilterBar>

  {#if groupError}
    <p class="error">
      <TriangleAlert size={16} strokeWidth={2} />
      {groupError}
    </p>
  {/if}

  <AsyncState
    loading={view.loading && !view.loaded}
    busy={view.loading && view.loaded}
    error={view.error}
    empty={view.items.length === 0}
    emptyMessage={view.hasActiveFilters
      ? "No images match your filters."
      : "No virtual images yet. Add one to start tracking tags."}
    columns={6}
  >
    {#snippet emptyAction()}
      {#if !view.hasActiveFilters}
        <Button onclick={() => push("/images/new")}>
          <Plus size={16} strokeWidth={2} />
          Add virtual image
        </Button>
      {/if}
    {/snippet}

    <div class="card table-wrap">
      <table class="table">
        <thead>
          <tr>
            <SortableHeader
              label="Name"
              direction={view.sortDirection("name")}
              onSort={() => view.toggleSort("name")}
            />
            <th>Tag</th>
            <SortableHeader
              label="Last pulled"
              class="phone-hidden time-cell"
              direction={view.sortDirection("lastPulled")}
              onSort={() => view.toggleSort("lastPulled")}
            />
            {#if rollouts.size > 0}<th class="phone-hidden">Containers</th>{/if}
            <SortableHeader
              label="Newer tag"
              direction={view.sortDirection("status")}
              onSort={() => view.toggleSort("status")}
            />
            {#if pullInfo}<th class="phone-hidden" aria-label="Copy pull command"></th>{/if}
          </tr>
        </thead>
        <tbody>
          {#each view.items as image (image.id)}
            {@const rollout = rollouts.get(image.id)}
            <tr class="clickable" onclick={(event) => openImage(event, image.id)}>
              <td>
                <span class="name-cell">
                  <a class="row-link" href={`#/images/${image.id}${rowQuery()}`}
                    >{image.name}</a
                  >
                  {#if image.lastCheckError}
                    <span
                      class="check-error-icon"
                      title={image.lastCheckError}
                      aria-label="Last tag check failed: {image.lastCheckError}"
                    >
                      <CircleAlert size={14} strokeWidth={2} />
                    </span>
                  {/if}
                  {#if image.group}
                    {@const groupId = image.group.id}
                    <button
                      type="button"
                      class="group-label"
                      title={`In group ${image.group.name}. Click to manage it.`}
                      onclick={() => openGroup(groupId)}
                    >
                      <Layers size={12} strokeWidth={2} />
                      <span class="group-label-name">{image.group.name}</span>
                    </button>
                  {/if}
                </span>
                <div class="source muted phone-hidden">
                  {#each imageParts(imageSource(image)) as part, index (index)}{part}<wbr />{/each}
                </div>
                {#if rollout}
                  <div class="phone-only source">{@render rolloutInfo(rollout)}</div>
                {/if}
              </td>
              <td>{image.tag}</td>
              <td class="phone-hidden time-cell">
                {#if image.lastPulledAt}
                  <span title={`${formatDate(image.lastPulledAt)} (${image.lastPulledTag})`}
                    >{formatRelativeTime(image.lastPulledAt)}</span
                  >
                {:else}
                  <span class="muted" title="No pull recorded yet">-</span>
                {/if}
              </td>
              {#if rollouts.size > 0}
                <td class="phone-hidden time-cell">
                  {#if rollout}
                    {@render rolloutInfo(rollout)}
                  {/if}
                </td>
              {/if}
              <td>
                {#if image.pinned}
                  <span class="muted pinned" title="Pinned: new versions aren't flagged or emailed">
                    <Pin size={12} strokeWidth={2} />
                    Pinned
                  </span>
                {:else if image.updateAvailable}
                  <StateIndicator
                    tone="warning"
                    label={image.updateAvailableTag ? `${image.updateAvailableTag} available` : "Newer tag available"}
                    title="{image.updateAvailableTag
                      ? `${image.updateAvailableTag} is newer than ${image.tag}. `
                      : ''}Change the tag on Updates or Image details."
                  />
                {/if}
              </td>
              {#if pullInfo}
                <td class="phone-hidden copy-cell row-reveal">
                  <CopyButton
                    text={pullCommand(pullInfo, image.name)}
                    label={`Copy pull command for ${image.name}`}
                    copiedMessage="Pull command copied to clipboard."
                  />
                </td>
              {/if}
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

{#if managedGroup}
  <ImageGroupModal
    open
    group={managedGroup}
    onClose={() => (managedGroup = null)}
    onSaved={(name) => handleGroupChanged(`Group "${name}" saved.`)}
    onDeleted={(name) => handleGroupChanged(`Group "${name}" deleted.`)}
  />
{/if}

<ConfirmDialog
  open={showRefreshAllConfirm}
  title="Refresh all tags"
  message="Check every virtual image's upstream registry for new or updated tags now? The check runs in the background and resets the interval until the next automatic check."
  confirmLabel="Refresh all"
  error={refreshAllError}
  submitting={refreshingAll}
  onConfirm={confirmRefreshAll}
  onCancel={cancelRefreshAll}
/>

<style>
  .source {
    margin-top: 2px;
    font-size: 0.8125rem;
    overflow-wrap: break-word;
    white-space: normal;
  }

  .pinned {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    white-space: nowrap;
  }

  .copy-cell {
    width: 1%;
    text-align: right;
  }

  .row-link {
    font-weight: 500;
    overflow-wrap: break-word;
  }

  .row-link:focus-visible {
    outline: 2px solid var(--color-accent);
    outline-offset: 2px;
    border-radius: 2px;
  }
</style>
