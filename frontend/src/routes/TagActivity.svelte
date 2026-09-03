<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import Activity from "@lucide/svelte/icons/activity";
  import X from "@lucide/svelte/icons/x";
  import AppShell from "../lib/components/AppShell.svelte";
  import PageTitle from "../lib/components/PageTitle.svelte";
  import AsyncState from "../lib/components/AsyncState.svelte";
  import Pagination from "../lib/components/Pagination.svelte";
  import FilterBar from "../lib/components/FilterBar.svelte";
  import DateRangeFilter from "../lib/components/DateRangeFilter.svelte";
  import { listTagEvents, TAG_ACTIVITY_PAGE_SIZE } from "../lib/api/events";
  import { getImage } from "../lib/api/images";
  import { errorMessage } from "../lib/api/client";
  import { TAG_EVENT_TYPES } from "../lib/api/types/events";
  import type { ImageEvent } from "../lib/api/types/events";
  import {
    formatDate,
    formatAuditType,
    toRfc3339DayStart,
    toRfc3339DayEnd,
  } from "../lib/utils/format";
  import {
    readListQuery,
    pushListQuery,
    watchListQuery,
  } from "../lib/utils/listQuery";

  const FILTER_DEFAULTS = {
    type: "",
    imageId: 0,
    since: "",
    until: "",
    offset: 0,
  };

  let events = $state<ImageEvent[]>([]);
  let total = $state(0);
  let offset = $state(0);
  let selectedType = $state("");
  let selectedImageId = $state(0);
  let filterImageName = $state("");
  let sinceDate = $state("");
  let untilDate = $state("");
  let loading = $state(true);
  let loaded = $state(false);
  let error = $state<string | null>(null);
  let loadToken = 0;

  let hasActiveFilters = $derived(
    selectedType !== "" ||
      selectedImageId !== 0 ||
      sinceDate !== "" ||
      untilDate !== "",
  );

  async function load() {
    const requestId = ++loadToken;
    loading = true;
    error = null;

    try {
      const result = await listTagEvents({
        offset,
        limit: TAG_ACTIVITY_PAGE_SIZE,
        type: selectedType || undefined,
        imageId: selectedImageId || undefined,
        since: toRfc3339DayStart(sinceDate),
        until: toRfc3339DayEnd(untilDate),
      });
      if (requestId !== loadToken) return;
      events = result.events;
      total = result.total;
    } catch (err) {
      if (requestId !== loadToken) return;
      error = errorMessage(err, "Failed to load tag activity");
    } finally {
      if (requestId === loadToken) {
        loading = false;
        loaded = true;
      }
    }
  }

  async function resolveImageName(id: number) {
    if (id === 0) {
      filterImageName = "";
      return;
    }

    try {
      filterImageName = (await getImage(id)).name;
    } catch {
      filterImageName = "";
    }
  }

  function syncFromUrl() {
    const filters = readListQuery(FILTER_DEFAULTS);
    selectedType = filters.type;
    selectedImageId = filters.imageId;
    sinceDate = filters.since;
    untilDate = filters.until;
    offset = filters.offset;
    resolveImageName(selectedImageId);
    load();
  }

  function syncToUrl() {
    pushListQuery("/tag-activity", {
      type: selectedType,
      imageId: selectedImageId,
      since: sinceDate,
      until: untilDate,
      offset,
    });
  }

  onMount(() => watchListQuery(syncFromUrl));

  function handleOffsetChange(newOffset: number) {
    offset = newOffset;
    syncToUrl();
    load();
  }

  function handleFilterChange() {
    offset = 0;
    syncToUrl();
    load();
  }

  function clearImageFilter() {
    selectedImageId = 0;
    filterImageName = "";
    handleFilterChange();
  }

  function clearFilters() {
    selectedType = "";
    selectedImageId = 0;
    filterImageName = "";
    sinceDate = "";
    untilDate = "";
    offset = 0;
    syncToUrl();
    load();
  }
</script>

<PageTitle title="Tag activity" />

<AppShell>
  <div class="title-row">
    <Activity size={20} strokeWidth={1.75} />
    <h1>Tag activity</h1>
  </div>

  <FilterBar active={hasActiveFilters} onClear={clearFilters}>
    <div class="filter-field">
      <span class="filter-label">Event</span>
      <select
        class="input filter-control"
        class:is-active={selectedType !== ""}
        bind:value={selectedType}
        onchange={handleFilterChange}
      >
        <option value="">All events</option>
        {#each TAG_EVENT_TYPES as type (type)}
          <option value={type}>{formatAuditType(type)}</option>
        {/each}
      </select>
    </div>

    <div class="filter-field">
      <span class="filter-label">Date</span>
      <DateRangeFilter
        bind:since={sinceDate}
        bind:until={untilDate}
        onChange={handleFilterChange}
      />
    </div>

    {#if selectedImageId !== 0}
      <span class="active-filter">
        <span class="active-filter-label">
          Image: {filterImageName || `#${selectedImageId}`}
        </span>
        <button
          type="button"
          class="active-filter-dismiss"
          aria-label="Clear image filter"
          onclick={clearImageFilter}
        >
          <X size={13} strokeWidth={2.5} />
        </button>
      </span>
    {/if}
  </FilterBar>

  <AsyncState
    loading={loading && !loaded}
    busy={loading && loaded}
    {error}
    empty={events.length === 0}
    emptyMessage="No tag activity yet."
    columns={4}
  >
    <div class="card table-wrap">
      <table class="table">
        <thead>
          <tr>
            <th>Time</th>
            <th>Event</th>
            <th>Image</th>
            <th>Tags</th>
          </tr>
        </thead>
        <tbody>
          {#each events as event (event.id)}
            <tr>
              <td>{formatDate(event.createdAt)}</td>
              <td>{formatAuditType(event.type)}</td>
              <td>
                <a
                  class="row-link"
                  href={`/images/${event.imageId}`}
                  use:link
                >
                  {event.imageName}
                </a>
              </td>
              <td class="tags-cell">{event.data.tags.join(", ")}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <Pagination
      {total}
      limit={TAG_ACTIVITY_PAGE_SIZE}
      {offset}
      onOffsetChange={handleOffsetChange}
    />
  </AsyncState>
</AppShell>

<style>
  .title-row {
    margin-bottom: var(--space-4);
  }

  .active-filter {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    padding: var(--space-1) var(--space-1) var(--space-1) var(--space-3);
    border: 1px solid var(--color-accent);
    border-radius: var(--radius-full);
    background: var(--color-accent-muted-bg);
    font-size: 0.8125rem;
  }

  .active-filter-label {
    color: var(--color-text);
  }

  .active-filter-dismiss {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 2px;
    border: none;
    border-radius: var(--radius-full);
    background: none;
    color: var(--color-text-muted);
    cursor: pointer;
  }

  .active-filter-dismiss:hover {
    color: var(--color-text);
  }

  .row-link {
    color: var(--color-accent);
    text-decoration: none;
  }

  .row-link:hover {
    text-decoration: underline;
  }

  .tags-cell {
    font-family: ui-monospace, monospace;
    font-size: 0.8125rem;
    word-break: break-word;
  }
</style>
