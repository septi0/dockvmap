<script lang="ts">
  import { onMount } from "svelte";
  import AsyncState from "../AsyncState.svelte";
  import Pagination from "../Pagination.svelte";
  import FilterBar from "../FilterBar.svelte";
  import DateRangeFilter from "../DateRangeFilter.svelte";
  import { listFailures, FAILURES_PAGE_SIZE } from "../../api/failures";
  import {
    FAILURE_SOURCES,
    FAILURE_SOURCE_LABELS,
  } from "../../api/types/failures";
  import type { Failure } from "../../api/types/failures";
  import {
    formatDate,
    toRfc3339DayStart,
    toRfc3339DayEnd,
  } from "../../utils/format";
  import { createListView } from "../../utils/listView.svelte";

  function sourceLabel(source: string) {
    return FAILURE_SOURCE_LABELS[source] ?? source;
  }

  const view = createListView<
    { source: string; since: string; until: string; offset: number },
    Failure
  >({
    routePath: "/system/failures",
    defaults: { source: "", since: "", until: "", offset: 0 },
    errorFallback: "Failed to load failures",
    fetch: (q) =>
      listFailures({
        offset: q.offset,
        limit: FAILURES_PAGE_SIZE,
        source: q.source || undefined,
        since: toRfc3339DayStart(q.since),
        until: toRfc3339DayEnd(q.until),
      }).then((r) => ({ items: r.failures, total: r.total })),
  });

  onMount(view.init);
</script>

<FilterBar active={view.hasActiveFilters} onClear={view.clear}>
  <div class="filter-field">
    <span class="filter-label">Source</span>
    <select
      class="input filter-control"
      class:is-active={view.filters.source !== ""}
      bind:value={view.filters.source}
      onchange={view.applyFilters}
    >
      <option value="">All sources</option>
      {#each FAILURE_SOURCES as source (source)}
        <option value={source}>{sourceLabel(source)}</option>
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
</FilterBar>

<AsyncState
  loading={view.loading && !view.loaded}
  busy={view.loading && view.loaded}
  error={view.error}
  empty={view.items.length === 0}
  emptyMessage="No background failures in the last 30 days."
  columns={3}
>
  <div class="card table-wrap">
    <table class="table">
      <thead>
        <tr>
          <th>Time</th>
          <th>Source</th>
          <th>Message</th>
        </tr>
      </thead>
      <tbody>
        {#each view.items as failure, i (i)}
          <tr>
            <td class="time-cell">{formatDate(failure.occurredAt)}</td>
            <td>{sourceLabel(failure.source)}</td>
            <td class="message-cell">{failure.message}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>

  <Pagination
    total={view.total}
    limit={FAILURES_PAGE_SIZE}
    offset={view.filters.offset}
    onOffsetChange={view.setOffset}
  />
</AsyncState>

<style>
  .time-cell {
    white-space: nowrap;
  }

  .message-cell {
    color: var(--color-danger);
    overflow-wrap: anywhere;
  }
</style>
