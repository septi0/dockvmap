<script lang="ts">
  import Boxes from "@lucide/svelte/icons/boxes";
  import CircleArrowUp from "@lucide/svelte/icons/circle-arrow-up";
  import CircleX from "@lucide/svelte/icons/circle-x";
  import ArrowLeftRight from "@lucide/svelte/icons/arrow-left-right";
  import RefreshCw from "@lucide/svelte/icons/refresh-cw";
  import HardDrive from "@lucide/svelte/icons/hard-drive";
  import StatTile from "./StatTile.svelte";
  import {
    formatDate,
    formatNumber,
    formatRelativeTime,
  } from "../../utils/format";
  import type {
    DashboardHosts,
    DashboardSummary,
  } from "../../api/types/dashboard";
  import type { ProxyMetrics } from "../../api/types/metrics";
  import type { DashboardSectionView } from "../../services/dashboard";
  import type { TagRefreshStatusState } from "../../services/tagRefreshStatus";

  let {
    summary,
    metrics,
    hosts,
    tagRefresh,
  }: {
    summary: DashboardSectionView<DashboardSummary>;
    metrics: DashboardSectionView<ProxyMetrics>;
    hosts: DashboardSectionView<DashboardHosts>;
    tagRefresh: TagRefreshStatusState;
  } = $props();

  let hostCounts = $derived(hosts.data);

  let hostsCaption = $derived.by(() => {
    if (!hostCounts) return "";

    const parts = [];
    const needAttention = hostCounts.total - hostCounts.online;

    if (needAttention > 0) parts.push(`${needAttention} need attention`);
    if (hostCounts.outdated > 0) parts.push(`${hostCounts.outdated} behind`);

    return parts.join(" · ");
  });

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
      return { value: "-", tone: "neutral" as const, caption: undefined };

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

<div class="stat-strip" class:busy-dim={busy}>
  <StatTile
    label="Tracked images"
    value={counts ? formatNumber(counts.images.total) : "-"}
    href="/images"
    {loading}
  >
    {#snippet icon()}<Boxes size={13} strokeWidth={1.75} />{/snippet}
  </StatTile>

  <StatTile
    label="Updates available"
    value={counts ? formatNumber(counts.images.updateAvailable) : "-"}
    tone={counts && counts.images.updateAvailable > 0 ? "accent" : "neutral"}
    href="/updates"
    {loading}
  >
    {#snippet icon()}<CircleArrowUp size={13} strokeWidth={1.75} />{/snippet}
  </StatTile>

  <StatTile
    label="Failed checks"
    value={counts ? formatNumber(counts.images.failedCheck) : "-"}
    tone={counts && counts.images.failedCheck > 0 ? "danger" : "neutral"}
    href="/images?status=failedCheck"
    {loading}
  >
    {#snippet icon()}<CircleX size={13} strokeWidth={1.75} />{/snippet}
  </StatTile>

  <StatTile
    label="Image pulls · 7d"
    href="/images?sort=lastPulled&order=desc"
    value={week ? formatNumber(week.manifestRequests) : "-"}
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
    href="/system/tasks"
    value={lastCheck.value}
    tone={lastCheck.tone}
    valueStyle="badge"
    caption={lastCheck.caption}
    title={checkTimes}
    {loading}
  >
    {#snippet icon()}<RefreshCw size={13} strokeWidth={1.75} />{/snippet}
  </StatTile>

  {#if hostCounts && hostCounts.total > 0}
    <StatTile
      label="Hosts"
      value={`${hostCounts.online} / ${hostCounts.total}`}
      tone={hostCounts.online < hostCounts.total ? "warning" : "neutral"}
      caption={hostsCaption || "all online"}
      captionTone={hostsCaption ? "warning" : "muted"}
      title={hostCounts.outdated > 0
        ? "Running containers that aren't on the current version yet"
        : undefined}
      href="/hosts"
      {loading}
    >
      {#snippet icon()}<HardDrive size={13} strokeWidth={1.75} />{/snippet}
    </StatTile>
  {/if}
</div>

<style>
  .stat-strip {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
    gap: var(--space-4);
    transition: opacity var(--transition-fast);
  }

  @media (max-width: 1499px) {
    .stat-strip {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }
  }

  @media (max-width: 768px) {
    .stat-strip {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
</style>
