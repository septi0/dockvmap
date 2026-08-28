<script lang="ts">
  import { onMount } from "svelte";
  import { push } from "svelte-spa-router";
  import Plus from "@lucide/svelte/icons/plus";
  import Package from "@lucide/svelte/icons/package";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Check from "@lucide/svelte/icons/check";
  import AppShell from "../lib/components/AppShell.svelte";
  import PageTitle from "../lib/components/PageTitle.svelte";
  import AsyncState from "../lib/components/AsyncState.svelte";
  import Pagination from "../lib/components/Pagination.svelte";
  import FilterBar from "../lib/components/FilterBar.svelte";
  import SearchInput from "../lib/components/SearchInput.svelte";
  import Button from "../lib/components/Button.svelte";
  import { listImages, IMAGES_PAGE_SIZE } from "../lib/api/images";
  import { ApiError } from "../lib/api/client";
  import type { Image } from "../lib/api/types/images";
  import { formatDate } from "../lib/utils/format";
  import {
    readListQuery,
    writeListQuery,
    pushListQuery,
    watchListQuery,
  } from "../lib/utils/listQuery";

  const FILTER_DEFAULTS = { search: "", updateAvailable: false, offset: 0 };

  let images = $state<Image[]>([]);
  let total = $state(0);
  let offset = $state(0);
  let search = $state("");
  let updateAvailable = $state(false);
  let loading = $state(true);
  let loaded = $state(false);
  let error = $state<string | null>(null);
  let loadToken = 0;

  async function load() {
    const requestId = ++loadToken;
    loading = true;
    error = null;

    try {
      const result = await listImages({
        offset,
        limit: IMAGES_PAGE_SIZE,
        search: search || undefined,
        updateAvailable: updateAvailable ? true : undefined,
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
    updateAvailable = filters.updateAvailable;
    offset = filters.offset;
    load();
  }

  function syncToUrl() {
    pushListQuery("/images", { search, updateAvailable, offset });
  }

  onMount(() => watchListQuery(syncFromUrl));

  function imageHref(id: number) {
    return `#/images/${id}${writeListQuery({ search, updateAvailable, offset })}`;
  }

  function openImage(event: MouseEvent, id: number) {
    if ((event.target as HTMLElement).closest("a")) return;
    push(`/images/${id}${writeListQuery({ search, updateAvailable, offset })}`);
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

  function handleUpdateAvailableToggle() {
    offset = 0;
    syncToUrl();
    load();
  }

  function clearFilters() {
    search = "";
    updateAvailable = false;
    offset = 0;
    syncToUrl();
    load();
  }
</script>

<PageTitle title="Virtual Images" />

<AppShell>
  <div class="header">
    <div class="title-row">
      <Package size={20} strokeWidth={1.75} />
      <h1>Virtual Images</h1>
    </div>
    <Button onclick={() => push("/images/new")}>
      <Plus size={16} strokeWidth={2} />
      Add virtual image
    </Button>
  </div>

  <FilterBar
    active={search !== "" || updateAvailable}
    onClear={clearFilters}
  >
    <SearchInput
      bind:value={search}
      placeholder="Search images…"
      onSearch={handleSearch}
    />
    <label class="checkbox">
      <input
        type="checkbox"
        bind:checked={updateAvailable}
        onchange={handleUpdateAvailableToggle}
      />
      <span>Updates available only</span>
    </label>
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

    <div class="card">
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
                <a class="row-link" href={imageHref(image.id)}>{image.name}</a>
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

<style>
  td {
    vertical-align: middle;
  }

  .header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: var(--space-4);
  }

  .clickable {
    cursor: pointer;
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
