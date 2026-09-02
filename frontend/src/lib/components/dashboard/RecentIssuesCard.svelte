<script lang="ts">
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import CircleCheck from "@lucide/svelte/icons/circle-check";
  import DashboardCard from "./DashboardCard.svelte";
  import CardBody from "./CardBody.svelte";
  import CardState from "./CardState.svelte";
  import { listRecentFailures } from "../../api/failures";
  import { ApiError } from "../../api/client";
  import { dashboardRefresh } from "../../services/dashboardRefresh";
  import type { RecentFailure } from "../../api/types/failures";
  import { formatDate, formatRelativeTime } from "../../utils/format";

  const CARD_ID = "recent-issues";
  const SHOWN = 5;

  let failures = $state<RecentFailure[]>([]);
  let loading = $state(true);
  let loaded = $state(false);
  let error = $state<string | null>(null);

  let refreshNonce = $derived($dashboardRefresh.nonce);

  $effect(() => {
    refreshNonce;
    void load();
  });

  async function load() {
    dashboardRefresh.begin(CARD_ID);
    loading = true;
    error = null;

    try {
      failures = await listRecentFailures();
    } catch (err) {
      error =
        err instanceof ApiError ? err.message : "Failed to load recent issues";
    } finally {
      loading = false;
      loaded = true;
      dashboardRefresh.end(CARD_ID, !error);
    }
  }

  let shown = $derived(failures.slice(0, SHOWN));
</script>

<DashboardCard title="Recent issues">
  {#snippet icon()}<TriangleAlert size={16} strokeWidth={1.75} />{/snippet}

  <CardBody
    loading={loading && !loaded}
    busy={loading && loaded}
    hasError={!!error}
    empty={failures.length === 0}
  >
    {#snippet errorState()}
      <CardState
        tone="error"
        title="Couldn’t load recent issues"
        description={error ?? undefined}
      >
        {#snippet icon()}<TriangleAlert size={30} strokeWidth={1.5} />{/snippet}
        {#snippet action()}
          <button
            type="button"
            class="link"
            onclick={() => dashboardRefresh.requestRefresh()}
          >
            Try again
          </button>
        {/snippet}
      </CardState>
    {/snippet}

    {#snippet emptyState()}
      <CardState
        tone="success"
        title="No recent issues"
        description="Notifications, tag refresh, and discovery have logged no failures in the last 30 days."
      >
        {#snippet icon()}<CircleCheck size={30} strokeWidth={1.5} />{/snippet}
      </CardState>
    {/snippet}

    <ul class="issue-list">
      {#each shown as failure, i (i)}
        <li>
          <span class="issue-date muted" title={formatDate(failure.occurredAt)}>
            {formatRelativeTime(failure.occurredAt)}
          </span>
          <span class="issue-message" title={failure.message}>
            {failure.message}
          </span>
        </li>
      {/each}
    </ul>

    {#if failures.length > SHOWN}
      <p class="issue-more muted">
        {failures.length - SHOWN} more in the last 30 days
      </p>
    {/if}
  </CardBody>
</DashboardCard>

<style>
  .issue-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .issue-list li {
    display: flex;
    flex-direction: column;
    gap: 2px;
    font-size: 0.8125rem;
  }

  .issue-date {
    font-size: 0.75rem;
    white-space: nowrap;
  }

  .issue-message {
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 3;
    line-clamp: 3;
    overflow: hidden;
    overflow-wrap: anywhere;
    color: var(--color-danger);
  }

  .issue-more {
    margin: var(--space-4) 0 0;
    font-size: 0.75rem;
  }
</style>
