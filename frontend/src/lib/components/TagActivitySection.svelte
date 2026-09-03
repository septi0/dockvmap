<script lang="ts">
  import { link } from "svelte-spa-router";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import { listTagEvents } from "../api/events";
  import { errorMessage } from "../api/client";
  import {
    formatAuditType,
    formatDate,
    formatNumber,
    formatRelativeTime,
  } from "../utils/format";
  import type { ImageEvent } from "../api/types/events";

  let {
    imageId,
    reloadSignal = 0,
  }: { imageId: number; reloadSignal?: number } = $props();

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
    imageId;
    reloadSignal;
    load();
  });
</script>

<div class="card section-card">
  <h2>Upstream tag activity</h2>
  <p class="section-hint muted">
    Upstream tag changes DockVMap has detected for this image: tags added, tags
    removed, and upgrades becoming available.
  </p>

  {#if error}
    <p class="error">
      <TriangleAlert size={16} strokeWidth={2} />
      {error}
    </p>
  {:else if loading}
    <p class="muted">Loading…</p>
  {:else if events.length === 0}
    <p class="muted">No upstream tag changes recorded for this image yet.</p>
  {:else}
    <ul class="activity-list">
      {#each events as event (event.id)}
        <li>
          <span class="activity-main">
            <span class="activity-type">{formatAuditType(event.type)}</span>
            <span class="activity-tags">{event.data.tags.join(", ")}</span>
          </span>
          <span class="activity-time muted" title={formatDate(event.createdAt)}>
            {formatRelativeTime(event.createdAt)}
          </span>
        </li>
      {/each}
    </ul>

    {#if total > events.length}
      <a
        class="view-all link"
        href={`/tag-activity?imageId=${imageId}`}
        use:link
      >
        View all {formatNumber(total)} events
      </a>
    {/if}
  {/if}
</div>

<style>
  .activity-list {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .activity-list li {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-3);
    padding: var(--space-2) 0;
    border-bottom: 1px solid var(--color-border);
  }

  .activity-list li:last-child {
    border-bottom: none;
  }

  .activity-main {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
    min-width: 0;
  }

  .activity-type {
    font-size: 0.875rem;
    font-weight: 500;
    white-space: nowrap;
  }

  .activity-tags {
    font-family: ui-monospace, monospace;
    font-size: 0.8125rem;
    color: var(--color-text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .activity-time {
    flex-shrink: 0;
    font-size: 0.8125rem;
  }

  .view-all {
    display: inline-block;
    margin-top: var(--space-3);
  }
</style>
