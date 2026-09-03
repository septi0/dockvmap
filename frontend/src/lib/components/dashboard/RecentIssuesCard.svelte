<script lang="ts">
  import { link } from "svelte-spa-router";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import CircleCheck from "@lucide/svelte/icons/circle-check";
  import DashboardCard from "./DashboardCard.svelte";
  import CardBody from "./CardBody.svelte";
  import CardState from "./CardState.svelte";
  import CardErrorState from "./CardErrorState.svelte";
  import {
    formatDate,
    formatNumber,
    formatRelativeTime,
  } from "../../utils/format";
  import type { DashboardIssues } from "../../api/types/dashboard";
  import type { DashboardCardProps } from "./types";

  const SHOWN = 5;

  let {
    data,
    error,
    loading,
    busy,
    onRetry,
  }: DashboardCardProps<DashboardIssues> = $props();

  let failures = $derived(data?.failures ?? []);
  let total = $derived(data?.total ?? 0);
  let shown = $derived(failures.slice(0, SHOWN));
</script>

<DashboardCard title="Recent issues">
  {#snippet icon()}<TriangleAlert size={16} strokeWidth={1.75} />{/snippet}

  <CardBody {loading} {busy} hasError={!!error} empty={failures.length === 0}>
    {#snippet errorState()}
      <CardErrorState title="Couldn’t load recent issues" {error} {onRetry} />
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

    {#if total > shown.length}
      <a class="view-all link" href="/system/failures" use:link>
        View all {formatNumber(total)} issues
      </a>
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

</style>
