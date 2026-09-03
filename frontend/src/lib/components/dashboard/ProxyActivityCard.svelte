<script lang="ts">
  import ArrowLeftRight from "@lucide/svelte/icons/arrow-left-right";
  import DashboardCard from "./DashboardCard.svelte";
  import CardBody from "./CardBody.svelte";
  import CardErrorState from "./CardErrorState.svelte";
  import { formatBytes, formatNumber } from "../../utils/format";
  import type {
    ProxyMetrics,
    ProxyMetricsWindow,
  } from "../../api/types/metrics";
  import type { DashboardCardProps } from "./types";

  const WINDOWS: { value: ProxyMetricsWindow; label: string }[] = [
    { value: "last7d", label: "7 days" },
    { value: "last30d", label: "30 days" },
  ];

  let {
    data,
    error,
    loading,
    busy,
    onRetry,
  }: DashboardCardProps<ProxyMetrics> = $props();

  let activeWindow = $state<ProxyMetricsWindow>("last7d");

  let view = $derived(data ? data.windows[activeWindow] : null);
  let cache = $derived(data?.cache ?? null);

  let failureRateLabel = $derived.by(() => {
    if (!view || view.upstreamFailures === 0 || view.totalRequests === 0)
      return null;
    const rate = (view.upstreamFailures / view.totalRequests) * 100;
    if (rate < 0.1) return "<0.1%";
    return `${rate.toFixed(rate < 10 ? 1 : 0)}%`;
  });

  let cacheHitRate = $derived.by(() => {
    if (!view) return null;
    const total = view.cacheHits + view.cacheMisses;
    if (total === 0) return null;
    return Math.round((view.cacheHits / total) * 100);
  });

  let cachePct = $derived.by(() => {
    if (!cache || cache.maxBytes <= 0) return null;
    return Math.min(100, Math.round((cache.usedBytes / cache.maxBytes) * 100));
  });
</script>

<DashboardCard title="Proxy activity">
  {#snippet icon()}<ArrowLeftRight size={16} strokeWidth={1.75} />{/snippet}

  {#snippet action()}
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
  {/snippet}

  <CardBody {loading} {busy} hasError={!!error}>
    {#snippet errorState()}
      <CardErrorState title="Couldn’t load proxy metrics" {error} {onRetry} />
    {/snippet}

    {#if view}
      <div class="stat-grid">
        <div class="stat-tile">
          <span class="stat-value">{formatNumber(view.manifestRequests)}</span>
          <span class="stat-label muted">Image pulls</span>
        </div>

        <div class="stat-tile">
          <span class="stat-value">{formatNumber(view.blobRequests)}</span>
          <span class="stat-label muted">Layer downloads</span>
        </div>

        <div class="stat-tile">
          <span class="stat-value" class:danger={view.upstreamFailures > 0}>
            {formatNumber(view.upstreamFailures)}
          </span>
          <span class="stat-label muted">
            Upstream failures{failureRateLabel ? ` · ${failureRateLabel}` : ""}
          </span>
        </div>

        {#if cacheHitRate !== null}
          <div class="stat-tile">
            <span class="stat-value">{cacheHitRate}%</span>
            <span class="stat-label muted">Cache hit rate</span>
          </div>
        {/if}

        {#if cache}
          <div class="stat-tile">
            <span class="stat-value">{formatBytes(cache.usedBytes)}</span>
            {#if cachePct !== null}
              <div
                class="cache-bar"
                role="img"
                aria-label="{cachePct}% of cache limit used"
              >
                <div
                  class="cache-bar-fill"
                  class:high={cachePct >= 90}
                  style:width="{cachePct}%"
                ></div>
              </div>
              <span class="stat-label muted">
                Cache on disk · {cachePct}% of {formatBytes(cache.maxBytes)}
              </span>
            {:else}
              <span class="stat-label muted">Cache on disk</span>
            {/if}
          </div>
        {/if}
      </div>
    {/if}
  </CardBody>
</DashboardCard>

<style>
  .stat-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
    gap: var(--space-5) var(--space-6);
  }

  .stat-tile {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .stat-value {
    font-size: 1.75rem;
    font-weight: 600;
    line-height: 1.15;
    letter-spacing: -0.02em;
  }

  .stat-value.danger {
    color: var(--color-danger);
  }

  .stat-label {
    font-size: 0.75rem;
    font-weight: 500;
    letter-spacing: 0.02em;
  }

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

  .cache-bar {
    height: 4px;
    border-radius: var(--radius-full);
    background: var(--color-surface-hover);
    overflow: hidden;
  }

  .cache-bar-fill {
    height: 100%;
    border-radius: inherit;
    background: var(--color-accent);
    transition: width var(--transition);
  }

  .cache-bar-fill.high {
    background: var(--color-warning);
  }
</style>
