<script lang="ts">
  import { onMount } from "svelte";
  import AsyncState from "../AsyncState.svelte";
  import Pagination from "../Pagination.svelte";
  import FilterBar from "../FilterBar.svelte";
  import DateRangeFilter from "../DateRangeFilter.svelte";
  import Modal from "../Modal.svelte";
  import DetailRow from "../DetailRow.svelte";
  import DeviceIcon from "../DeviceIcon.svelte";
  import { listAuditLogs, AUDIT_LOG_PAGE_SIZE } from "../../api/audit";
  import { errorMessage } from "../../api/client";
  import { AUDIT_TYPES } from "../../api/types/audit";
  import type { AuditLog } from "../../api/types/audit";
  import {
    formatDate,
    formatAuditType,
    toRfc3339DayStart,
    toRfc3339DayEnd,
  } from "../../utils/format";
  import { parseUserAgent } from "../../utils/userAgent";
  import {
    readListQuery,
    pushListQuery,
    watchListQuery,
  } from "../../utils/listQuery";

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

  async function load() {
    const requestId = ++loadToken;
    loading = true;
    error = null;

    try {
      const result = await listAuditLogs({
        offset,
        limit: AUDIT_LOG_PAGE_SIZE,
        type: selectedType || undefined,
        since: toRfc3339DayStart(sinceDate),
        until: toRfc3339DayEnd(untilDate),
      });
      if (requestId !== loadToken) return;
      auditLogs = result.auditLogs;
      total = result.total;
    } catch (err) {
      if (requestId !== loadToken) return;
      error = errorMessage(err, "Failed to load audit log");
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
    pushListQuery("/system/audit", {
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
    <DetailRow label="User agent">
      {#if selectedEntry.userAgent}
        {@const ua = parseUserAgent(selectedEntry.userAgent)}
        <span class="ua-line">
          <span class="icon"><DeviceIcon device={ua.device} /></span>
          {ua.label}
        </span>
        <span class="ua-raw muted">{selectedEntry.userAgent}</span>
      {:else}
        -
      {/if}
    </DetailRow>

    {#if selectedEntry.data && Object.keys(selectedEntry.data).length > 0}
      <DetailRow label="Details">
        <pre class="data">{JSON.stringify(selectedEntry.data, null, 2)}</pre>
      </DetailRow>
    {/if}
  {/if}
</Modal>

<style>
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

  .ua-line {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .ua-line .icon {
    display: inline-flex;
    flex-shrink: 0;
    color: var(--color-text-muted);
  }

  .ua-raw {
    display: block;
    margin-top: 2px;
    font-size: 0.8125rem;
    word-break: break-word;
  }
</style>
