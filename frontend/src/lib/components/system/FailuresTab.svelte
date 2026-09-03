<script lang="ts">
  import { onMount } from "svelte";
  import AsyncState from "../AsyncState.svelte";
  import Pagination from "../Pagination.svelte";
  import FilterBar from "../FilterBar.svelte";
  import DateRangeFilter from "../DateRangeFilter.svelte";
  import { listFailures, FAILURES_PAGE_SIZE } from "../../api/failures";
  import { errorMessage } from "../../api/client";
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
  import {
    readListQuery,
    pushListQuery,
    watchListQuery,
  } from "../../utils/listQuery";

  const FILTER_DEFAULTS = {
    source: "",
    since: "",
    until: "",
    offset: 0,
  };

  let failures = $state<Failure[]>([]);
  let total = $state(0);
  let offset = $state(0);
  let selectedSource = $state("");
  let sinceDate = $state("");
  let untilDate = $state("");
  let loading = $state(true);
  let loaded = $state(false);
  let error = $state<string | null>(null);
  let loadToken = 0;

  let hasActiveFilters = $derived(
    selectedSource !== "" || sinceDate !== "" || untilDate !== "",
  );

  function sourceLabel(source: string) {
    return FAILURE_SOURCE_LABELS[source] ?? source;
  }

  async function load() {
    const requestId = ++loadToken;
    loading = true;
    error = null;

    try {
      const result = await listFailures({
        offset,
        limit: FAILURES_PAGE_SIZE,
        source: selectedSource || undefined,
        since: toRfc3339DayStart(sinceDate),
        until: toRfc3339DayEnd(untilDate),
      });
      if (requestId !== loadToken) return;
      failures = result.failures;
      total = result.total;
    } catch (err) {
      if (requestId !== loadToken) return;
      error = errorMessage(err, "Failed to load failures");
    } finally {
      if (requestId === loadToken) {
        loading = false;
        loaded = true;
      }
    }
  }

  function syncFromUrl() {
    const filters = readListQuery(FILTER_DEFAULTS);
    selectedSource = filters.source;
    sinceDate = filters.since;
    untilDate = filters.until;
    offset = filters.offset;
    load();
  }

  function syncToUrl() {
    pushListQuery("/system/failures", {
      source: selectedSource,
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

  function clearFilters() {
    selectedSource = "";
    sinceDate = "";
    untilDate = "";
    offset = 0;
    syncToUrl();
    load();
  }
</script>

<FilterBar active={hasActiveFilters} onClear={clearFilters}>
  <div class="filter-field">
    <span class="filter-label">Source</span>
    <select
      class="input filter-control"
      class:is-active={selectedSource !== ""}
      bind:value={selectedSource}
      onchange={handleFilterChange}
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
      bind:since={sinceDate}
      bind:until={untilDate}
      onChange={handleFilterChange}
    />
  </div>
</FilterBar>

<AsyncState
  loading={loading && !loaded}
  busy={loading && loaded}
  {error}
  empty={failures.length === 0}
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
        {#each failures as failure, i (i)}
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
    {total}
    limit={FAILURES_PAGE_SIZE}
    {offset}
    onOffsetChange={handleOffsetChange}
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
