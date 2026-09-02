<script lang="ts">
  import Boxes from "@lucide/svelte/icons/boxes";
  import CircleArrowUp from "@lucide/svelte/icons/circle-arrow-up";
  import CircleX from "@lucide/svelte/icons/circle-x";
  import ArrowLeftRight from "@lucide/svelte/icons/arrow-left-right";
  import RefreshCw from "@lucide/svelte/icons/refresh-cw";
  import StatTile from "./StatTile.svelte";
  import BusyOverlay from "./BusyOverlay.svelte";
  import {
    formatDate,
    formatNumber,
    formatRelativeTime,
  } from "../../utils/format";
  import type { DashboardSummary } from "../../api/types/dashboard";
  import type { ProxyMetrics } from "../../api/types/metrics";
  import type { DashboardSectionView } from "../../stores/dashboard";
  import type { TagRefreshStatusState } from "../../stores/tagRefreshStatus";

  let {
    summary,
    metrics,
    tagRefresh,
  }: {
    summary: DashboardSectionView<DashboardSummary>;
    metrics: DashboardSectionView<ProxyMetrics>;
    tagRefresh: TagRefreshStatusState;
  } = $props();

  let counts = $derived(summary.data);
  let loading = $derived(summary.loading);
  let busy = $derived(summary.busy);

  let check = $derived(tagRefresh.unavailable ? null : tagRefresh.data);

  let refreshOverdue = $derived(
    !!check &&
      !check.running &&
      !!check.nextDue &&
      new Date(check.nextDue).getTime() <= Date.now(),
  );

  let lastCheck = $derived.by(() => {
    if (tagRefresh.unavailable) {
      return {
        value: "Unknown",
        tone: "neutral" as const,
        caption: "check status unavailable",
      };
    }

    if (!check)
      return { value: "—", tone: "neutral" as const, caption: undefined };

    if (!check.enabled) {
      return {
        value: "Off",
        tone: "neutral" as const,
        caption: "set tags_check_interval",
      };
    }

    if (check.running) {
      return {
        value: "Running",
        tone: "accent" as const,
        caption: "checking now",
      };
    }

    const last = check.lastRun
      ? `last ${formatRelativeTime(check.lastRun)}`
      : "never run";

    if (refreshOverdue) {
      return { value: "Overdue", tone: "warning" as const, caption: last };
    }

    const next = check.nextDue
      ? `next ${formatRelativeTime(check.nextDue)}`
      : null;

    return {
      value: "On schedule",
      tone: "neutral" as const,
      caption: next ? `${last} · ${next}` : last,
    };
  });

  let checkTimes = $derived.by(() => {
    if (!check || !check.enabled) return undefined;

    const parts: string[] = [];

    if (check.lastRun) parts.push(`Last run: ${formatDate(check.lastRun)}`);
    if (check.nextDue) parts.push(`Next due: ${formatDate(check.nextDue)}`);

    return parts.length > 0 ? parts.join("\n") : undefined;
  });

  let week = $derived(metrics.data?.windows.last7d ?? null);
</script>

<div class="stat-strip-wrap">
  <BusyOverlay active={busy} />

  <div class="stat-strip" class:is-busy={busy}>
    <StatTile
      label="Tracked images"
      value={counts ? formatNumber(counts.images.total) : "—"}
      href="/images"
      {loading}
    >
      {#snippet icon()}<Boxes size={13} strokeWidth={1.75} />{/snippet}
    </StatTile>

    <StatTile
      label="Updates available"
      value={counts ? formatNumber(counts.images.updateAvailable) : "—"}
      tone={counts && counts.images.updateAvailable > 0 ? "accent" : "neutral"}
      href="/images?status=updateAvailable"
      {loading}
    >
      {#snippet icon()}<CircleArrowUp size={13} strokeWidth={1.75} />{/snippet}
    </StatTile>

    <StatTile
      label="Failed checks"
      value={counts ? formatNumber(counts.images.failedCheck) : "—"}
      tone={counts && counts.images.failedCheck > 0 ? "danger" : "neutral"}
      href="/images?status=failedCheck"
      {loading}
    >
      {#snippet icon()}<CircleX size={13} strokeWidth={1.75} />{/snippet}
    </StatTile>

    <StatTile
      label="Proxy requests · 7d"
      value={week ? formatNumber(week.totalRequests) : "—"}
      caption={week && week.upstreamFailures > 0
        ? `${formatNumber(week.upstreamFailures)} upstream failures`
        : undefined}
      captionTone={week && week.upstreamFailures > 0 ? "danger" : "muted"}
      {loading}
    >
      {#snippet icon()}<ArrowLeftRight size={13} strokeWidth={1.75} />{/snippet}
    </StatTile>

    <StatTile
      label="Tag checks"
      value={lastCheck.value}
      tone={lastCheck.tone}
      valueStyle="badge"
      caption={lastCheck.caption}
      title={checkTimes}
      {loading}
    >
      {#snippet icon()}<RefreshCw size={13} strokeWidth={1.75} />{/snippet}
    </StatTile>
  </div>
</div>

<style>
  .stat-strip-wrap {
    position: relative;
  }

  .stat-strip {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
    gap: var(--space-4);
    transition: opacity var(--transition-fast);
  }

  .stat-strip.is-busy {
    opacity: 0.55;
    pointer-events: none;
  }
</style>
