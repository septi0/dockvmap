<script lang="ts">
  import { onMount, untrack } from "svelte";
  import { push } from "svelte-spa-router";
  import Plus from "@lucide/svelte/icons/plus";
  import Package from "@lucide/svelte/icons/package";
  import RefreshCw from "@lucide/svelte/icons/refresh-cw";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import CircleAlert from "@lucide/svelte/icons/circle-alert";
  import Check from "@lucide/svelte/icons/check";
  import AppShell from "../lib/components/AppShell.svelte";
  import PageTitle from "../lib/components/PageTitle.svelte";
  import AsyncState from "../lib/components/AsyncState.svelte";
  import Pagination from "../lib/components/Pagination.svelte";
  import FilterBar from "../lib/components/FilterBar.svelte";
  import SearchInput from "../lib/components/SearchInput.svelte";
  import Button from "../lib/components/Button.svelte";
  import ConfirmDialog from "../lib/components/ConfirmDialog.svelte";
  import { listImages, IMAGES_PAGE_SIZE } from "../lib/api/images";
  import { triggerTagRefresh } from "../lib/api/worker";
  import { toast } from "../lib/services/toast";
  import { tagRefreshStatus } from "../lib/services/tagRefreshStatus";
  import { ApiError } from "../lib/api/client";
  import {
    IMAGE_STATUS_FILTERS,
    type Image,
    type ImageStatusFilter,
  } from "../lib/api/types/images";
  import { formatDate } from "../lib/utils/format";
  import {
    readListQuery,
    writeListQuery,
    pushListQuery,
    watchListQuery,
  } from "../lib/utils/listQuery";

  const FILTER_DEFAULTS = { search: "", status: "", offset: 0 };

  const STATUS_OPTIONS: { value: ImageStatusFilter | ""; label: string }[] = [
    { value: "", label: "All images" },
    { value: "updateAvailable", label: "Updates available" },
    { value: "failedCheck", label: "Failed checks" },
  ];

  let images = $state<Image[]>([]);
  let total = $state(0);
  let offset = $state(0);
  let search = $state("");
  let status = $state<ImageStatusFilter | "">("");
  let loading = $state(true);
  let loaded = $state(false);
  let error = $state<string | null>(null);
  let loadToken = 0;

  let showRefreshAllConfirm = $state(false);
  let refreshingAll = $state(false);
  let refreshAllError = $state<string | null>(null);

  let tagRefreshRunning = $derived($tagRefreshStatus.data?.running ?? false);
  let wasTagRefreshRunning = false;

  function toStatusFilter(value: string): ImageStatusFilter | "" {
    return (IMAGE_STATUS_FILTERS as readonly string[]).includes(value)
      ? (value as ImageStatusFilter)
      : "";
  }

  async function load() {
    const requestId = ++loadToken;
    loading = true;
    error = null;

    try {
      const result = await listImages({
        offset,
        limit: IMAGES_PAGE_SIZE,
        search: search || undefined,
        status: status || undefined,
      });
      if (requestId !== loadToken) return;
      images = result.images;
      total = result.total;
    } catch (err) {
      if (requestId !== loadToken) return;
      error = err instanceof ApiError ? err.message : "Failed to load images";
    } finally {
      if (requestId === loadToken) {
        loading = false;
        loaded = true;
      }
    }
  }

  function syncFromUrl() {
    const filters = readListQuery(FILTER_DEFAULTS);
    search = filters.search;
    status = toStatusFilter(filters.status);
    offset = filters.offset;
    load();
  }

  function syncToUrl() {
    pushListQuery("/images", { search, status, offset });
  }

  onMount(() => watchListQuery(syncFromUrl));
  onMount(() => tagRefreshStatus.watch());

  // reload the table once a background sweep finishes so tags/badges aren't stale
  $effect(() => {
    const running = tagRefreshRunning;
    untrack(() => {
      if (wasTagRefreshRunning && !running && loaded) load();
      wasTagRefreshRunning = running;
    });
  });

  function imageHref(id: number) {
    return `#/images/${id}${writeListQuery({ search, status, offset })}`;
  }

  function openImage(event: MouseEvent, id: number) {
    if ((event.target as HTMLElement).closest("a")) return;
    push(`/images/${id}${writeListQuery({ search, status, offset })}`);
  }

  function handleOffsetChange(newOffset: number) {
    offset = newOffset;
    syncToUrl();
    load();
  }

  function handleSearch(value: string) {
    search = value;
    offset = 0;
    syncToUrl();
    load();
  }

  function handleStatusChange() {
    offset = 0;
    syncToUrl();
    load();
  }

  function clearFilters() {
    search = "";
    status = "";
    offset = 0;
    syncToUrl();
    load();
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
      refreshAllError =
        err instanceof ApiError ? err.message : "Failed to start tag check";
      tagRefreshStatus.refresh();
    } finally {
      refreshingAll = false;
    }
  }
</script>

<PageTitle title="Virtual Images" />

<AppShell>
  <div class="list-header">
    <div class="title-row">
      <Package size={20} strokeWidth={1.75} />
      <h1>Virtual Images</h1>
    </div>
    <div class="header-actions">
      <Button
        variant="secondary"
        onclick={openRefreshAll}
        disabled={tagRefreshRunning || refreshingAll}
      >
        <RefreshCw size={16} strokeWidth={2} />
        {tagRefreshRunning ? "Checking tags…" : "Refresh all tags"}
      </Button>
      <Button onclick={() => push("/images/new")}>
        <Plus size={16} strokeWidth={2} />
        Add virtual image
      </Button>
    </div>
  </div>

  <FilterBar active={search !== "" || status !== ""} onClear={clearFilters}>
    <SearchInput
      bind:value={search}
      placeholder="Search images…"
      onSearch={handleSearch}
    />
    <div class="filter-field">
      <span class="filter-label">Status</span>
      <select
        class="input filter-control"
        class:is-active={status !== ""}
        bind:value={status}
        onchange={handleStatusChange}
      >
        {#each STATUS_OPTIONS as option (option.value)}
          <option value={option.value}>{option.label}</option>
        {/each}
      </select>
    </div>
  </FilterBar>

  <AsyncState
    loading={loading && !loaded}
    busy={loading && loaded}
    {error}
    empty={images.length === 0}
    emptyMessage="No virtual images yet. Add one to start tracking tags."
    columns={6}
  >
    {#snippet emptyAction()}
      <Button onclick={() => push("/images/new")}>
        <Plus size={16} strokeWidth={2} />
        Add virtual image
      </Button>
    {/snippet}

    <div class="card table-wrap">
      <table class="table">
        <thead>
          <tr>
            <th>Name</th>
            <th>Registry</th>
            <th>Repository</th>
            <th>Tag</th>
            <th>Last checked</th>
            <th>Update</th>
          </tr>
        </thead>
        <tbody>
          {#each images as image (image.id)}
            <tr class="clickable" onclick={(event) => openImage(event, image.id)}>
              <td>
                <span class="name-cell">
                  <a class="row-link" href={imageHref(image.id)}>{image.name}</a>
                  {#if image.lastCheckError}
                    <span
                      class="check-error-icon"
                      title={image.lastCheckError}
                      aria-label="Last tag check failed: {image.lastCheckError}"
                    >
                      <CircleAlert size={14} strokeWidth={2} />
                    </span>
                  {/if}
                </span>
              </td>
              <td>{image.registry}</td>
              <td>{image.repository}</td>
              <td>{image.tag}</td>
              <td>{formatDate(image.lastChecked, "Never")}</td>
              <td>
                {#if image.updateAvailable}
                  <span
                    class="badge badge-warning"
                    title={image.updateAvailableTag
                      ? `Update to ${image.updateAvailableTag}`
                      : undefined}
                  >
                    <TriangleAlert size={12} strokeWidth={2} />
                    {image.updateAvailableTag
                      ? `Update to ${image.updateAvailableTag}`
                      : "Update available"}
                  </span>
                {:else}
                  <span class="badge">
                    <Check size={12} strokeWidth={2} />
                    Up to date
                  </span>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <Pagination
      {total}
      limit={IMAGES_PAGE_SIZE}
      {offset}
      onOffsetChange={handleOffsetChange}
    />
  </AsyncState>
</AppShell>

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
  .header-actions {
    display: flex;
    gap: var(--space-2);
  }

  .clickable {
    cursor: pointer;
  }

  .name-cell {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
  }

  .check-error-icon {
    display: inline-flex;
    color: var(--color-danger);
  }

  .row-link {
    color: inherit;
    text-decoration: none;
    font-weight: 500;
  }

  .row-link:hover {
    text-decoration: underline;
  }

  .row-link:focus-visible {
    outline: 2px solid var(--color-accent);
    outline-offset: 2px;
    border-radius: 2px;
  }
</style>
