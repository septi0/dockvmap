<script lang="ts">
  import { onMount } from "svelte";
  import { push } from "svelte-spa-router";
  import Plus from "@lucide/svelte/icons/plus";
  import Package from "@lucide/svelte/icons/package";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Check from "@lucide/svelte/icons/check";
  import AppShell from "../lib/components/AppShell.svelte";
  import AsyncState from "../lib/components/AsyncState.svelte";
  import Pagination from "../lib/components/Pagination.svelte";
  import FilterBar from "../lib/components/FilterBar.svelte";
  import SearchInput from "../lib/components/SearchInput.svelte";
  import Button from "../lib/components/Button.svelte";
  import { listImages, IMAGES_PAGE_SIZE } from "../lib/api/images";
  import { ApiError } from "../lib/api/client";
  import type { Image } from "../lib/api/types/images";
  import { formatDate } from "../lib/utils/format";

  let images = $state<Image[]>([]);
  let total = $state(0);
  let offset = $state(0);
  let search = $state("");
  let updateAvailableOnly = $state(false);
  let loading = $state(true);
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
        updateAvailable: updateAvailableOnly ? true : undefined,
      });
      if (requestId !== loadToken) return;
      images = result.images;
      total = result.total;
    } catch (err) {
      if (requestId !== loadToken) return;
      error = err instanceof ApiError ? err.message : "Failed to load images";
    } finally {
      if (requestId === loadToken) loading = false;
    }
  }

  function applyFilterFromQuery() {
    const query = window.location.hash.split("?")[1];
    updateAvailableOnly = new URLSearchParams(query).get("updateAvailable") === "true";
    offset = 0;
    load();
  }

  onMount(() => {
    applyFilterFromQuery();
    window.addEventListener("hashchange", applyFilterFromQuery);
    return () => window.removeEventListener("hashchange", applyFilterFromQuery);
  });

  function handleOffsetChange(newOffset: number) {
    offset = newOffset;
    load();
  }

  function handleSearch(value: string) {
    search = value;
    offset = 0;
    load();
  }

  function handleUpdateAvailableToggle() {
    offset = 0;
    load();
  }

  function clearFilters() {
    search = "";
    updateAvailableOnly = false;
    offset = 0;
    load();
  }
</script>

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
    active={search !== "" || updateAvailableOnly}
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
        bind:checked={updateAvailableOnly}
        onchange={handleUpdateAvailableToggle}
      />
      <span>Updates available only</span>
    </label>
  </FilterBar>

  <AsyncState
    {loading}
    {error}
    empty={images.length === 0}
    emptyMessage="No virtual images yet. Add one to start tracking tags."
    columns={6}
  >
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
            <tr
              class="clickable"
              tabindex="0"
              onclick={() => push(`/images/${image.id}`)}
              onkeydown={(event) =>
                event.key === "Enter" && push(`/images/${image.id}`)}
            >
              <td>{image.name}</td>
              <td>{image.registry}</td>
              <td>{image.repository}</td>
              <td>{image.tag}</td>
              <td>{formatDate(image.lastChecked, "Never")}</td>
              <td>
                {#if image.updateAvailable}
                  <span class="badge badge-warning">
                    <TriangleAlert size={12} strokeWidth={2} />
                    Update available
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
</style>
