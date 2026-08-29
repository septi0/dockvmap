<script lang="ts">
  import LayoutDashboard from "@lucide/svelte/icons/layout-dashboard";
  import AppShell from "../lib/components/AppShell.svelte";
  import PageTitle from "../lib/components/PageTitle.svelte";
  import RecentIssuesCard from "../lib/components/dashboard/RecentIssuesCard.svelte";
  import UpdatesAvailableCard from "../lib/components/dashboard/UpdatesAvailableCard.svelte";
  import RecentActivityCard from "../lib/components/dashboard/RecentActivityCard.svelte";
  import ProxyActivityCard from "../lib/components/dashboard/ProxyActivityCard.svelte";
  import TagChecksCard from "../lib/components/dashboard/TagChecksCard.svelte";
</script>

<PageTitle title="Dashboard" />

<AppShell>
  <div class="page-header">
    <div class="title-row">
      <LayoutDashboard size={20} strokeWidth={1.75} />
      <h1>Dashboard</h1>
    </div>
  </div>

  <div class="dashboard-grid">
    <RecentIssuesCard />
    <UpdatesAvailableCard />
    <RecentActivityCard />
    <ProxyActivityCard />
    <TagChecksCard />
  </div>
</AppShell>

<style>
  .dashboard-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
    gap: var(--space-4);
    align-items: start;
  }
</style>
