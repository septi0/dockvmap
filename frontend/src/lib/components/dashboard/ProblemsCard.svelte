<script lang="ts">
  import { link, push } from "svelte-spa-router";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import CircleCheck from "@lucide/svelte/icons/circle-check";
  import DashboardCard from "./DashboardCard.svelte";
  import CardBody from "./CardBody.svelte";
  import CardState from "./CardState.svelte";
  import CardErrorState from "./CardErrorState.svelte";
  import HostStatus from "../hosts/HostStatus.svelte";
  import { formatCount, formatDate, formatNumber, formatRelativeTime } from "../../utils/format";
  import type { DashboardIssues } from "../../api/types/dashboard";
  import type { Host } from "../../api/types/hosts";
  import type { DashboardCardProps } from "./types";

  const SHOWN = 5;

  let {
    data,
    error,
    loading,
    busy,
    onRetry,
    hosts,
    hasHosts,
  }: DashboardCardProps<DashboardIssues> & { hosts: Host[]; hasHosts: boolean } = $props();

  let shownHosts = $derived(hosts.slice(0, SHOWN));
  let failures = $derived((data?.failures ?? []).slice(0, Math.max(0, SHOWN - shownHosts.length)));
  let total = $derived(data?.total ?? 0);
  let distinct = $derived(data?.distinct ?? 0);
</script>

<DashboardCard title="Problems" count={hosts.length + distinct}>
  {#snippet icon()}<TriangleAlert size={16} strokeWidth={1.75} />{/snippet}

  <CardBody {loading} {busy} hasError={!!error} empty={hosts.length === 0 && total === 0}>
    {#snippet errorState()}
      <CardErrorState title="Couldn’t load problems" {error} {onRetry} />
    {/snippet}

    {#snippet emptyState()}
      <CardState
        tone="success"
        title="No problems"
        description={hasHosts
          ? "Every host answers, and nothing failed in the last 30 days."
          : "Nothing failed in the last 30 days."}
      >
        {#snippet icon()}<CircleCheck size={30} strokeWidth={1.5} />{/snippet}
      </CardState>
    {/snippet}

    <ul class="item-list">
      {#each shownHosts as host (host.id)}
        <li>
          <button type="button" class="item-row" onclick={() => push(`/hosts/${host.id}`)}>
            <span class="item-main">
              <span class="item-title">{host.name}</span>
              <HostStatus {host} showLastSeen inline />
            </span>
          </button>
        </li>
      {/each}
      {#each failures as failure, i (i)}
        <li class="failure">
          <span class="issue-date muted" title={formatDate(failure.occurredAt)}>
            {formatRelativeTime(failure.occurredAt)}{failure.occurrences > 1
              ? ` · ${formatCount(failure.occurrences, "time")}`
              : ""}
          </span>
          <span class="issue-message" title={failure.message}>{failure.message}</span>
          {#if !failure.message.includes(failure.error)}
            <span class="full-error" title={failure.error}>full error</span>
          {/if}
        </li>
      {/each}
    </ul>

    {#if hosts.length > shownHosts.length}
      <a class="view-all link" href="/hosts" use:link>View all {hosts.length} hosts</a>
    {/if}
    {#if total > failures.length}
      <a class="view-all link" href="/system/failures" use:link>View all {formatNumber(total)} failures</a>
    {/if}
  </CardBody>
</DashboardCard>

<style>
  .failure {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: var(--space-2) 0;
    border-bottom: 1px solid var(--color-border);
    font-size: 0.8125rem;
  }

  .failure:last-child {
    border-bottom: none;
  }

  .issue-date {
    font-size: 0.75rem;
    white-space: nowrap;
  }

  .full-error {
    align-self: flex-start;
    color: var(--color-text-muted);
    font-size: 0.75rem;
    text-decoration: underline dotted;
    cursor: help;
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
</style>
