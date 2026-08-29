<script lang="ts">
  import { onMount } from "svelte";
  import ArrowLeftRight from "@lucide/svelte/icons/arrow-left-right";
  import AsyncState from "../AsyncState.svelte";
  import { getProxyMetrics } from "../../api/metrics";
  import { ApiError } from "../../api/client";
  import type {
    ProxyMetrics,
    ProxyMetricsWindow,
  } from "../../api/types/metrics";
  import { formatBytes } from "../../utils/format";

  const WINDOWS: { value: ProxyMetricsWindow; label: string }[] = [
    { value: "today", label: "Today" },
    { value: "last7d", label: "7 days" },
    { value: "last30d", label: "30 days" },
  ];

  let metrics = $state<ProxyMetrics | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let activeWindow = $state<ProxyMetricsWindow>("last7d");

  let view = $derived(metrics ? metrics.windows[activeWindow] : null);

  let cacheHitRate = $derived.by(() => {
    if (!view) return null;
    const total = view.cacheHits + view.cacheMisses;
    if (total === 0) return null;
    return `${Math.round((view.cacheHits / total) * 100)}%`;
  });

  let cacheUsageLabel = $derived.by(() => {
    if (!metrics?.cache) return null;
    const { usedBytes, maxBytes } = metrics.cache;
    return maxBytes > 0
      ? `${formatBytes(usedBytes)} / ${formatBytes(maxBytes)}`
      : formatBytes(usedBytes);
  });

  async function load() {
    loading = true;
    error = null;

    try {
      metrics = await getProxyMetrics();
    } catch (err) {
      error =
        err instanceof ApiError ? err.message : "Failed to load proxy metrics";
    } finally {
      loading = false;
    }
  }

  onMount(load);
</script>

<div class="card section-card">
  <div class="card-head">
    <div class="card-head-title">
      <ArrowLeftRight size={16} strokeWidth={1.75} />
      <h2>Proxy activity</h2>
    </div>

    <div class="segmented">
      {#each WINDOWS as option (option.value)}
        <button
          type="button"
          class:active={activeWindow === option.value}
          onclick={() => (activeWindow = option.value)}
        >
          {option.label}
        </button>
      {/each}
    </div>
  </div>

  <AsyncState {loading} {error}>
    {#if view}
      <div class="stat-grid">
        <div class="stat-tile">
          <span class="stat-value">{view.totalRequests}</span>
          <span class="stat-label muted">Total requests</span>
        </div>

        {#if cacheHitRate}
          <div class="stat-tile">
            <span class="stat-value">{cacheHitRate}</span>
            <span class="stat-label muted">Cache hit rate</span>
          </div>
        {/if}

        <div class="stat-tile">
          <span class="stat-value" class:danger={view.upstreamFailures > 0}>
            {view.upstreamFailures}
          </span>
          <span class="stat-label muted">Upstream failures</span>
        </div>
      </div>

      {#if cacheUsageLabel}
        <p class="stat-caption muted">Cache {cacheUsageLabel}</p>
      {/if}
    {/if}
  </AsyncState>
</div>

<style>
  .segmented {
    display: inline-flex;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    overflow: hidden;
  }

  .segmented button {
    padding: var(--space-1) var(--space-2);
    border: none;
    border-left: 1px solid var(--color-border);
    background: none;
    font: inherit;
    font-size: 0.75rem;
    font-weight: 500;
    color: var(--color-text-muted);
    cursor: pointer;
    transition:
      color var(--transition-fast),
      background-color var(--transition-fast);
  }

  .segmented button:first-child {
    border-left: none;
  }

  .segmented button:hover {
    color: var(--color-text);
    background: var(--color-surface-hover);
  }

  .segmented button.active {
    color: var(--color-accent);
    background: var(--color-accent-muted-bg);
  }
</style>
