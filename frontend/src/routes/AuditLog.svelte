<script lang="ts">
  import { onMount } from "svelte";
  import ScrollText from "@lucide/svelte/icons/scroll-text";
  import AppShell from "../lib/components/AppShell.svelte";
  import PageTitle from "../lib/components/PageTitle.svelte";
  import AsyncState from "../lib/components/AsyncState.svelte";
  import Pagination from "../lib/components/Pagination.svelte";
  import FilterBar from "../lib/components/FilterBar.svelte";
  import DateRangeFilter from "../lib/components/DateRangeFilter.svelte";
  import Modal from "../lib/components/Modal.svelte";
  import DetailRow from "../lib/components/DetailRow.svelte";
  import { listAuditLogs, AUDIT_LOG_PAGE_SIZE } from "../lib/api/audit";
  import { ApiError } from "../lib/api/client";
  import { AUDIT_TYPES } from "../lib/api/types/audit";
  import type { AuditLog } from "../lib/api/types/audit";
  import { formatDate, formatAuditType } from "../lib/utils/format";
  import {
    readListQuery,
    pushListQuery,
    watchListQuery,
  } from "../lib/utils/listQuery";

  const FILTER_DEFAULTS = {
    type: "",
    since: "",
    until: "",
    offset: 0,
  };

  let auditLogs = $state<AuditLog[]>([]);
  let total = $state(0);
  let offset = $state(0);
  let selectedType = $state("");
  let sinceDate = $state("");
  let untilDate = $state("");
  let loading = $state(true);
  let loaded = $state(false);
  let error = $state<string | null>(null);
  let selectedEntry = $state<AuditLog | null>(null);
  let loadToken = 0;

  let hasActiveFilters = $derived(
    selectedType !== "" || sinceDate !== "" || untilDate !== "",
  );

  function toRfc3339Since(date: string) {
    return date ? new Date(`${date}T00:00:00`).toISOString() : undefined;
  }

  function toRfc3339Until(date: string) {
    return date ? new Date(`${date}T23:59:59`).toISOString() : undefined;
  }

  async function load() {
    const requestId = ++loadToken;
    loading = true;
    error = null;

    try {
      const result = await listAuditLogs({
        offset,
        limit: AUDIT_LOG_PAGE_SIZE,
        type: selectedType || undefined,
        since: toRfc3339Since(sinceDate),
        until: toRfc3339Until(untilDate),
      });
      if (requestId !== loadToken) return;
      auditLogs = result.auditLogs;
      total = result.total;
    } catch (err) {
      if (requestId !== loadToken) return;
      error =
        err instanceof ApiError ? err.message : "Failed to load audit log";
    } finally {
      if (requestId === loadToken) {
        loading = false;
        loaded = true;
      }
    }
  }

  function syncFromUrl() {
    const filters = readListQuery(FILTER_DEFAULTS);
    selectedType = filters.type;
    sinceDate = filters.since;
    untilDate = filters.until;
    offset = filters.offset;
    load();
  }

  function syncToUrl() {
    pushListQuery("/audit-log", {
      type: selectedType,
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

  function selectRow(event: MouseEvent, entry: AuditLog) {
    if ((event.target as HTMLElement).closest("button")) return;
    selectedEntry = entry;
  }

  function clearFilters() {
    selectedType = "";
    sinceDate = "";
    untilDate = "";
    offset = 0;
    syncToUrl();
    load();
  }
</script>

<PageTitle title="Audit Log" />

<AppShell>
  <div class="title-row">
    <ScrollText size={20} strokeWidth={1.75} />
    <h1>Audit Log</h1>
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
        {#each AUDIT_TYPES as type (type)}
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
  </FilterBar>

  <AsyncState
    loading={loading && !loaded}
    busy={loading && loaded}
    {error}
    empty={auditLogs.length === 0}
    emptyMessage="No audit events yet."
    columns={4}
  >
    <div class="card table-wrap">
      <table class="table">
        <thead>
          <tr>
            <th>Time</th>
            <th>Event</th>
            <th>User</th>
            <th>IP</th>
          </tr>
        </thead>
        <tbody>
          {#each auditLogs as entry (entry.id)}
            <tr class="clickable" onclick={(event) => selectRow(event, entry)}>
              <td>
                <button
                  type="button"
                  class="row-trigger"
                  onclick={() => (selectedEntry = entry)}
                >
                  {formatDate(entry.createdAt)}
                </button>
              </td>
              <td>{formatAuditType(entry.type)}</td>
              <td>{entry.username ?? "-"}</td>
              <td>{entry.ip ?? "-"}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <Pagination
      {total}
      limit={AUDIT_LOG_PAGE_SIZE}
      {offset}
      onOffsetChange={handleOffsetChange}
    />
  </AsyncState>
</AppShell>

<Modal
  open={selectedEntry !== null}
  onClose={() => (selectedEntry = null)}
  title="Audit event"
>
  {#if selectedEntry}
    <DetailRow label="Time">{formatDate(selectedEntry.createdAt)}</DetailRow>
    <DetailRow label="Event">{formatAuditType(selectedEntry.type)}</DetailRow>
    <DetailRow label="User">{selectedEntry.username ?? "System"}</DetailRow>
    <DetailRow label="IP">{selectedEntry.ip ?? "-"}</DetailRow>
    <DetailRow label="User agent">{selectedEntry.userAgent ?? "-"}</DetailRow>

    {#if selectedEntry.data && Object.keys(selectedEntry.data).length > 0}
      <DetailRow label="Details">
        <pre class="data">{JSON.stringify(selectedEntry.data, null, 2)}</pre>
      </DetailRow>
    {/if}
  {/if}
</Modal>

<style>
  .title-row {
    margin-bottom: var(--space-4);
  }

  .clickable {
    cursor: pointer;
  }

  .row-trigger {
    padding: 0;
    border: none;
    background: none;
    font: inherit;
    color: inherit;
    cursor: pointer;
    text-align: left;
  }

  .row-trigger:focus-visible {
    outline: 2px solid var(--color-accent);
    outline-offset: 2px;
    border-radius: 2px;
  }

  .data {
    margin: 0;
    padding: var(--space-3);
    background: var(--color-bg);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    font-family: ui-monospace, monospace;
    font-size: 0.8125rem;
    white-space: pre-wrap;
    word-break: break-word;
    overflow-x: auto;
  }
</style>
