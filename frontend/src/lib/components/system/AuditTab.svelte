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
  import { AUDIT_TYPES } from "../../api/types/audit";
  import type { AuditLog } from "../../api/types/audit";
  import {
    formatDate,
    formatAuditType,
    toRfc3339DayStart,
    toRfc3339DayEnd,
  } from "../../utils/format";
  import { parseUserAgent } from "../../utils/userAgent";
  import { createListView } from "../../utils/listView.svelte";

  let selectedEntry = $state<AuditLog | null>(null);

  const view = createListView<
    { type: string; since: string; until: string; offset: number },
    AuditLog
  >({
    routePath: "/system/audit",
    defaults: { type: "", since: "", until: "", offset: 0 },
    errorFallback: "Failed to load audit log",
    fetch: (q) =>
      listAuditLogs({
        offset: q.offset,
        limit: AUDIT_LOG_PAGE_SIZE,
        type: q.type || undefined,
        since: toRfc3339DayStart(q.since),
        until: toRfc3339DayEnd(q.until),
      }).then((r) => ({ items: r.auditLogs, total: r.total })),
  });

  onMount(view.init);

  function selectRow(event: MouseEvent, entry: AuditLog) {
    if ((event.target as HTMLElement).closest("button")) return;
    selectedEntry = entry;
  }
</script>

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
      {#each AUDIT_TYPES as type (type)}
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
</FilterBar>

<AsyncState
  loading={view.loading && !view.loaded}
  busy={view.loading && view.loaded}
  error={view.error}
  empty={view.items.length === 0}
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
        {#each view.items as entry (entry.id)}
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
    total={view.total}
    limit={AUDIT_LOG_PAGE_SIZE}
    offset={view.filters.offset}
    onOffsetChange={view.setOffset}
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
