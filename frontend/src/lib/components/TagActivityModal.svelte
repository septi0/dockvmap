<script lang="ts">
  import { link } from "svelte-spa-router";
  import Modal from "./Modal.svelte";
  import AsyncState from "./AsyncState.svelte";
  import { listTagEvents } from "../api/events";
  import { errorMessage } from "../api/client";
  import { formatAuditType, formatDate, formatNumber } from "../utils/format";
  import type { ImageEvent } from "../api/types/events";

  let {
    open,
    imageId,
    onClose,
  }: { open: boolean; imageId: number; onClose: () => void } = $props();

  const SHOWN = 10;

  let events = $state<ImageEvent[]>([]);
  let total = $state(0);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let loadToken = 0;

  async function load() {
    const requestId = ++loadToken;
    loading = true;
    error = null;

    try {
      const result = await listTagEvents({ offset: 0, limit: SHOWN, imageId });
      if (requestId !== loadToken) return;
      events = result.events;
      total = result.total;
    } catch (err) {
      if (requestId !== loadToken) return;
      error = errorMessage(err, "Failed to load tag activity");
    } finally {
      if (requestId === loadToken) loading = false;
    }
  }

  $effect(() => {
    if (!open) return;
    imageId;
    load();
  });
</script>

<Modal {open} {onClose} title="Upstream tag activity" size="lg">
  <p class="hint muted">
    Upstream tag changes DockVMap has detected for this image: tags added,
    removed, and upgrades becoming available. Most recent first.
  </p>

  <AsyncState
    {loading}
    {error}
    empty={events.length === 0}
    emptyVariant="text"
    emptyMessage="No upstream tag changes recorded for this image yet."
    listSkeleton
  >
    <ul class="activity-list">
      {#each events as event (event.id)}
        <li class="activity-entry">
          <div class="activity-main">
            <span class="activity-type">{formatAuditType(event.type)}</span>
          </div>
          <div class="activity-meta">
            <span class="activity-tags">{event.data.tags.join(", ")}</span>
            <span class="activity-date">{formatDate(event.createdAt)}</span>
          </div>
        </li>
      {/each}
    </ul>

    {#if total > events.length}
      <a
        class="view-all link"
        href={`/tag-activity?imageId=${imageId}`}
        use:link
        onclick={onClose}
      >
        View all {formatNumber(total)} events
      </a>
    {/if}
  </AsyncState>
</Modal>

<style>
  .hint {
    margin: 0 0 var(--space-4);
    font-size: 0.875rem;
  }

  .activity-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .activity-entry {
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
  }

  .activity-main {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .activity-type {
    font-size: 0.9375rem;
    font-weight: 600;
    color: var(--color-text);
  }

  .activity-meta {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-top: var(--space-1);
    color: var(--color-text-muted);
    font-size: 0.8125rem;
  }

  .activity-tags {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: monospace;
  }

  .activity-date {
    flex-shrink: 0;
    white-space: nowrap;
  }

  .activity-date::before {
    content: "\2022";
    margin-right: var(--space-2);
    color: var(--color-text-faint);
  }
</style>
