<script lang="ts">
  import { onMount } from "svelte";
  import { push, link } from "svelte-spa-router";
  import LayoutDashboard from "@lucide/svelte/icons/layout-dashboard";
  import ArrowUp from "@lucide/svelte/icons/arrow-up";
  import AppShell from "../lib/components/AppShell.svelte";
  import AsyncState from "../lib/components/AsyncState.svelte";
  import { listImages } from "../lib/api/images";
  import { listEvents } from "../lib/api/events";
  import { getProxyMetrics } from "../lib/api/metrics";
  import { listRecentFailures } from "../lib/api/failures";
  import { ApiError } from "../lib/api/client";
  import type { Image } from "../lib/api/types/images";
  import type { ImageEvent } from "../lib/api/types/events";
  import type { ProxyMetrics } from "../lib/api/types/metrics";
  import type { RecentFailure } from "../lib/api/types/failures";
  import { formatDate, formatAuditType } from "../lib/utils/format";

  const RECENT_LIMIT = 6;

  let updatesAvailable = $state<Image[]>([]);
  let updatesTotal = $state(0);
  let updatesLoading = $state(true);
  let updatesError = $state<string | null>(null);

  let events = $state<ImageEvent[]>([]);
  let eventsLoading = $state(true);
  let eventsError = $state<string | null>(null);

  let metrics = $state<ProxyMetrics | null>(null);
  let metricsLoading = $state(true);
  let metricsError = $state<string | null>(null);

  let failures = $state<RecentFailure[]>([]);
  let failuresLoading = $state(true);
  let failuresError = $state<string | null>(null);

  let cacheHitRate = $derived.by(() => {
    if (!metrics || !metrics.cacheEnabled) return null;
    const total = metrics.cacheHits + metrics.cacheMisses;
    if (total === 0) return null;
    return `${Math.round((metrics.cacheHits / total) * 100)}%`;
  });

  async function loadUpdatesAvailable() {
    updatesLoading = true;
    updatesError = null;

    try {
      const result = await listImages({
        offset: 0,
        limit: RECENT_LIMIT,
        updateAvailable: true,
      });
      updatesAvailable = result.images;
      updatesTotal = result.total;
    } catch (err) {
      updatesError =
        err instanceof ApiError ? err.message : "Failed to load updates";
    } finally {
      updatesLoading = false;
    }
  }

  async function loadRecentActivity() {
    eventsLoading = true;
    eventsError = null;

    try {
      const result = await listEvents(0);
      events = result.events.slice(0, RECENT_LIMIT);
    } catch (err) {
      eventsError =
        err instanceof ApiError
          ? err.message
          : "Failed to load recent activity";
    } finally {
      eventsLoading = false;
    }
  }

  async function loadMetrics() {
    metricsLoading = true;
    metricsError = null;

    try {
      metrics = await getProxyMetrics();
    } catch (err) {
      metricsError =
        err instanceof ApiError ? err.message : "Failed to load proxy metrics";
    } finally {
      metricsLoading = false;
    }
  }

  async function loadRecentFailures() {
    failuresLoading = true;
    failuresError = null;

    try {
      failures = await listRecentFailures();
    } catch (err) {
      failuresError =
        err instanceof ApiError ? err.message : "Failed to load recent issues";
    } finally {
      failuresLoading = false;
    }
  }

  onMount(() => {
    loadUpdatesAvailable();
    loadRecentActivity();
    loadMetrics();
    loadRecentFailures();
  });
</script>

<AppShell>
  <div class="page-header">
    <div class="title-row">
      <LayoutDashboard size={20} strokeWidth={1.75} />
      <h1>Dashboard</h1>
    </div>
  </div>

  <div class="dashboard-grid">
    <div class="card section-card issues-card">
      <h2>Recent issues</h2>

      <AsyncState
        loading={failuresLoading}
        error={failuresError}
        empty={failures.length === 0}
        emptyMessage="No issues since last restart."
      >
        <div class="table-scroll">
          <table class="table">
            <thead>
              <tr>
                <th>Date</th>
                <th>Issue</th>
              </tr>
            </thead>
            <tbody>
              {#each failures as failure (failure.occurredAt + failure.message)}
                <tr>
                  <td class="muted failure-date">{formatDate(failure.occurredAt)}</td>
                  <td>{failure.message}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </AsyncState>
    </div>

    <div class="card section-card">
      <h2>Updates available</h2>

      <AsyncState
        loading={updatesLoading}
        error={updatesError}
        empty={updatesAvailable.length === 0}
        emptyMessage="Everything is up to date."
      >
        <ul class="item-list">
          {#each updatesAvailable as image (image.id)}
            <li>
              <button
                type="button"
                class="item-row"
                onclick={() => push(`/images/${image.id}`)}
              >
                <span class="item-main">
                  <span class="item-title">{image.name}</span>
                  <span class="item-sub muted"
                    >{image.registry}/{image.repository}</span
                  >
                  {#if image.updateAvailableTag}
                    <span class="item-sub muted"
                      >{image.tag} &rarr; {image.updateAvailableTag}</span
                    >
                  {/if}
                </span>
                <span class="badge badge-warning">
                  <ArrowUp size={12} strokeWidth={2.5} />
                  Update
                </span>
              </button>
            </li>
          {/each}
        </ul>

        {#if updatesTotal > updatesAvailable.length}
          <a class="view-all link" href="/images?updateAvailable=true" use:link>
            View all {updatesTotal} images with updates available
          </a>
        {/if}
      </AsyncState>
    </div>

    <div class="card section-card">
      <h2>Recent activity</h2>

      <AsyncState
        loading={eventsLoading}
        error={eventsError}
        empty={events.length === 0}
        emptyMessage="No tag activity recorded yet."
      >
        <ul class="item-list">
          {#each events as event (event.id)}
            <li>
              <button
                type="button"
                class="item-row"
                onclick={() => push(`/images/${event.imageId}`)}
              >
                <span class="item-main">
                  <span class="item-title">{event.imageName}</span>
                  <span
                    class="item-sub muted"
                    title="{formatAuditType(event.type)}: {event.data.tags.join(
                      ', ',
                    )}"
                  >
                    {formatAuditType(event.type)}: {event.data.tags.join(", ")}
                  </span>
                </span>
                <span class="item-time muted"
                  >{formatDate(event.createdAt)}</span
                >
              </button>
            </li>
          {/each}
        </ul>
      </AsyncState>
    </div>

    <div class="card section-card">
      <h2>Proxy activity</h2>

      <AsyncState loading={metricsLoading} error={metricsError}>
        {#if metrics}
          <div class="stat-grid">
            <div class="stat-tile">
              <span class="stat-value">{metrics.totalRequests}</span>
              <span class="stat-label muted">Total requests</span>
            </div>

            {#if cacheHitRate}
              <div class="stat-tile">
                <span class="stat-value">{cacheHitRate}</span>
                <span class="stat-label muted">Cache hit rate</span>
              </div>
            {/if}

            <div class="stat-tile">
              <span
                class="stat-value"
                class:danger={metrics.upstreamFailures > 0}
              >
                {metrics.upstreamFailures}
              </span>
              <span class="stat-label muted">Upstream failures</span>
            </div>
          </div>

          <p class="stat-caption muted">
            Since {formatDate(metrics.startedAt)} (resets on restart)
          </p>
        {/if}
      </AsyncState>
    </div>
  </div>
</AppShell>

<style>
  .dashboard-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
    gap: var(--space-4);
    align-items: start;
  }

  .item-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    width: 100%;
    padding: var(--space-2) 0;
    border: none;
    border-bottom: 1px solid var(--color-border);
    background: none;
    font: inherit;
    text-align: left;
    cursor: pointer;
    color: inherit;
  }

  li:last-child .item-row {
    border-bottom: none;
  }

  .item-time {
    flex-shrink: 0;
    font-size: 0.8125rem;
  }

  .view-all {
    display: inline-block;
    margin-top: var(--space-3);
  }

  .stat-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
    gap: var(--space-4);
  }

  .stat-tile {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .stat-value {
    font-size: 1.75rem;
    font-weight: 600;
    line-height: 1.2;
  }

  .stat-value.danger {
    color: var(--color-danger);
  }

  .stat-label {
    font-size: 0.8125rem;
  }

  .stat-caption {
    margin: var(--space-4) 0 0;
    font-size: 0.8125rem;
  }

  .issues-card {
    grid-column: 1 / -1;
  }

  .table-scroll {
    overflow-x: auto;
  }

  .failure-date {
    white-space: nowrap;
  }
</style>
