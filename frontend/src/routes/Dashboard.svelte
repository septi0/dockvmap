<script lang="ts">
  import { onMount } from "svelte";
  import LayoutDashboard from "@lucide/svelte/icons/layout-dashboard";
  import RefreshCw from "@lucide/svelte/icons/refresh-cw";
  import AppShell from "../lib/components/AppShell.svelte";
  import PageTitle from "../lib/components/PageTitle.svelte";
  import StatStrip from "../lib/components/dashboard/StatStrip.svelte";
  import RecentIssuesCard from "../lib/components/dashboard/RecentIssuesCard.svelte";
  import UpdatesAvailableCard from "../lib/components/dashboard/UpdatesAvailableCard.svelte";
  import RecentTagActivityCard from "../lib/components/dashboard/RecentTagActivityCard.svelte";
  import ProxyActivityCard from "../lib/components/dashboard/ProxyActivityCard.svelte";
  import { dashboard } from "../lib/services/dashboard";
  import { dashboardSections } from "../lib/stores/dashboard";
  import { tagRefreshStatus } from "../lib/services/tagRefreshStatus";
  import { formatRelativeTime } from "../lib/utils/format";

  let now = $state(Date.now());

  onMount(() => {
    const stopDashboard = dashboard.start();
    const stopWatching = tagRefreshStatus.watch();
    const stopListening = tagRefreshStatus.onCompleted(() => {
      void dashboard.refresh();
    });
    const tick = setInterval(() => (now = Date.now()), 30_000);

    return () => {
      clearInterval(tick);
      stopListening();
      stopWatching();
      stopDashboard();
    };
  });

  let sections = $derived($dashboardSections);
  let settledAt = $derived($dashboard.settledAt);
  let onRetry = () => void dashboard.refresh();

  let hasErrors = $derived(
    Object.values(sections).some((section) => section.error !== null),
  );

  let statusLabel = $derived.by(() => {
    if (settledAt === null) return { text: "Loading…", warn: false };
    if (hasErrors) return { text: "Some data didn’t load", warn: true };

    const since = formatRelativeTime(settledAt, "-", Math.max(now, settledAt));

    return { text: `Updated ${since}`, warn: false };
  });
</script>

<PageTitle title="Dashboard" />

<AppShell>
  <div class="page-header dashboard-header">
    <div class="title-row">
      <LayoutDashboard size={20} strokeWidth={1.75} />
      <h1>Dashboard</h1>
    </div>

    <div class="header-actions">
      <span
        class="updated"
        class:warn={statusLabel.warn}
        class:muted={!statusLabel.warn}
        aria-live="polite"
      >
        {statusLabel.text}
      </span>
      <button
        type="button"
        class="icon-button bordered"
        onclick={onRetry}
        aria-label="Refresh dashboard"
        aria-busy={$dashboard.loading}
        title="Refresh"
      >
        <span class="icon" class:spin={$dashboard.loading}>
          <RefreshCw size={15} strokeWidth={2} />
        </span>
      </button>
    </div>
  </div>

  <div class="dashboard">
    <section class="dash-band">
      <h2 class="band-label">Overview</h2>
      <StatStrip
        summary={sections.summary}
        metrics={sections.metrics}
        tagRefresh={$tagRefreshStatus}
      />
    </section>

    <section class="dash-band">
      <h2 class="band-label">Needs attention</h2>
      <div class="band-grid needs-attention">
        <UpdatesAvailableCard
          {...sections.updates}
          trackedImages={sections.summary.data?.images.total ?? null}
          {onRetry}
        />
        <RecentIssuesCard {...sections.issues} {onRetry} />
      </div>
    </section>

    <section class="dash-band">
      <h2 class="band-label">Activity &amp; health</h2>
      <div class="band-stack">
        <ProxyActivityCard {...sections.metrics} {onRetry} />
        <RecentTagActivityCard {...sections.activity} {onRetry} />
      </div>
    </section>
  </div>
</AppShell>

<style>
  .dashboard-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .header-actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-shrink: 0;
  }

  .updated {
    font-size: 0.75rem;
  }

  .updated.warn {
    color: var(--color-warning);
  }

  .header-actions .icon {
    display: inline-flex;
  }

  .dashboard {
    display: flex;
    flex-direction: column;
    gap: var(--space-8);
  }

  .dash-band {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .band-label {
    margin: 0;
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--color-text-faint);
  }

  .band-grid {
    display: grid;
    gap: var(--space-4);
  }

  .needs-attention {
    grid-template-columns: minmax(0, 1.6fr) minmax(0, 1fr);
    align-items: stretch;
  }

  .band-stack {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  @media (max-width: 900px) {
    .needs-attention {
      grid-template-columns: 1fr;
    }
  }
</style>
