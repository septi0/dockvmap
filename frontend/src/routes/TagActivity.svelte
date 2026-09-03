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
  import { TAG_EVENT_TYPES } from "../lib/api/types/events";
  import type { ImageEvent } from "../lib/api/types/events";
  import {
    formatDate,
    formatAuditType,
    toRfc3339DayStart,
    toRfc3339DayEnd,
  } from "../lib/utils/format";
  import { createListView } from "../lib/utils/listView.svelte";

  let filterImageName = $state("");

  const view = createListView<
    {
      type: string;
      imageId: number;
      since: string;
      until: string;
      offset: number;
    },
    ImageEvent
  >({
    routePath: "/tag-activity",
    defaults: { type: "", imageId: 0, since: "", until: "", offset: 0 },
    errorFallback: "Failed to load tag activity",
    fetch: (q) =>
      listTagEvents({
        offset: q.offset,
        limit: TAG_ACTIVITY_PAGE_SIZE,
        type: q.type || undefined,
        imageId: q.imageId || undefined,
        since: toRfc3339DayStart(q.since),
        until: toRfc3339DayEnd(q.until),
      }).then((r) => ({ items: r.events, total: r.total })),
  });

  onMount(view.init);

  $effect(() => {
    const id = view.filters.imageId;

    if (id === 0) {
      filterImageName = "";
      return;
    }

    getImage(id)
      .then((img) => {
        filterImageName = img.name;
      })
      .catch(() => {
        filterImageName = "";
      });
  });

  function clearImageFilter() {
    view.filters.imageId = 0;
    view.applyFilters();
  }
</script>

<PageTitle title="Tag activity" />

<AppShell>
  <div class="title-row">
    <Activity size={20} strokeWidth={1.75} />
    <h1>Tag activity</h1>
  </div>

  <FilterBar active={view.hasActiveFilters} onClear={view.clear}>
    <div class="filter-field">
      <span class="filter-label">Event</span>
      <select
        class="input filter-control"
        class:is-active={view.filters.type !== ""}
        bind:value={view.filters.type}
        onchange={view.applyFilters}
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
        bind:since={view.filters.since}
        bind:until={view.filters.until}
        onChange={view.applyFilters}
      />
    </div>

    {#if view.filters.imageId !== 0}
      <span class="active-filter">
        <span class="active-filter-label">
          Image: {filterImageName || `#${view.filters.imageId}`}
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
    loading={view.loading && !view.loaded}
    busy={view.loading && view.loaded}
    error={view.error}
    empty={view.items.length === 0}
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
          {#each view.items as event (event.id)}
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
      total={view.total}
      limit={TAG_ACTIVITY_PAGE_SIZE}
      offset={view.filters.offset}
      onOffsetChange={view.setOffset}
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
