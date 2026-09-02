<script lang="ts">
  import { onMount } from "svelte";
  import Boxes from "@lucide/svelte/icons/boxes";
  import CircleArrowUp from "@lucide/svelte/icons/circle-arrow-up";
  import CircleX from "@lucide/svelte/icons/circle-x";
  import ArrowLeftRight from "@lucide/svelte/icons/arrow-left-right";
  import RefreshCw from "@lucide/svelte/icons/refresh-cw";
  import StatTile from "./StatTile.svelte";
  import BusyOverlay from "./BusyOverlay.svelte";
  import { dashboardSummary } from "../../services/dashboardSummary";
  import { proxyMetrics } from "../../services/proxyMetrics";
  import { tagRefreshStatus } from "../../services/tagRefreshStatus";
  import { dashboardRefresh } from "../../services/dashboardRefresh";
  import { formatDate, formatNumber, formatRelativeTime } from "../../utils/format";

  const CARD_ID = "stat-strip";

  let loaded = $state(false);
  let fetching = $state(false);

  onMount(() => tagRefreshStatus.watch());

  let refreshNonce = $derived($dashboardRefresh.nonce);

  $effect(() => {
    refreshNonce;
    void load();
  });

  async function load() {
    dashboardRefresh.begin(CARD_ID);
    fetching = true;
    let ok = false;

    try {
      const [summaryOk, metricsOk] = await Promise.all([
        dashboardSummary.load(),
        proxyMetrics.load(),
        tagRefreshStatus.refresh(),
      ]);

      ok = summaryOk && metricsOk;
    } finally {
      fetching = false;
      loaded = true;
      dashboardRefresh.end(CARD_ID, ok);
    }
  }

  let summary = $derived($dashboardSummary.data);
  let metrics = $derived($proxyMetrics.data);
  let refresh = $derived($tagRefreshStatus.data);

  let skeleton = $derived(fetching && !loaded);
  let busy = $derived(fetching && loaded);

  let refreshOverdue = $derived(
    !!refresh &&
      !refresh.running &&
      !!refresh.nextDue &&
      new Date(refresh.nextDue).getTime() <= Date.now(),
  );

  let lastCheck = $derived.by(() => {
    if (!refresh) return { value: "—", tone: "neutral" as const, caption: undefined };

    if (!refresh.enabled) {
      return {
        value: "Off",
        tone: "neutral" as const,
        caption: "set tags_check_interval",
      };
    }

    if (refresh.running) {
      return { value: "Running", tone: "accent" as const, caption: "checking now" };
    }

    const last = refresh.lastRun
      ? `last ${formatRelativeTime(refresh.lastRun)}`
      : "never run";

    if (refreshOverdue) {
      return { value: "Overdue", tone: "warning" as const, caption: last };
    }

    const next = refresh.nextDue
      ? `next ${formatRelativeTime(refresh.nextDue)}`
      : null;

    return {
      value: "On schedule",
      tone: "neutral" as const,
      caption: next ? `${last} · ${next}` : last,
    };
  });

  let checkTimes = $derived.by(() => {
    if (!refresh || !refresh.enabled) return undefined;

    const parts: string[] = [];

    if (refresh.lastRun) parts.push(`Last run: ${formatDate(refresh.lastRun)}`);
    if (refresh.nextDue) parts.push(`Next due: ${formatDate(refresh.nextDue)}`);

    return parts.length > 0 ? parts.join("\n") : undefined;
  });

  let week = $derived(metrics?.windows.last7d ?? null);
</script>

<div class="stat-strip-wrap">
  <BusyOverlay active={busy} />

  <div class="stat-strip" class:is-busy={busy}>
    <StatTile
      label="Tracked images"
      value={summary ? formatNumber(summary.images.total) : "—"}
      href="/images"
      loading={skeleton}
    >
      {#snippet icon()}<Boxes size={13} strokeWidth={1.75} />{/snippet}
    </StatTile>

    <StatTile
      label="Updates available"
      value={summary ? formatNumber(summary.images.updateAvailable) : "—"}
      tone={summary && summary.images.updateAvailable > 0 ? "accent" : "neutral"}
      href="/images?status=updateAvailable"
      loading={skeleton}
    >
      {#snippet icon()}<CircleArrowUp size={13} strokeWidth={1.75} />{/snippet}
    </StatTile>

    <StatTile
      label="Failed checks"
      value={summary ? formatNumber(summary.images.failedCheck) : "—"}
      tone={summary && summary.images.failedCheck > 0 ? "danger" : "neutral"}
      href="/images?status=failedCheck"
      loading={skeleton}
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
      loading={skeleton}
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
      loading={skeleton}
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
