<script lang="ts">
  import { push, link } from "svelte-spa-router";
  import Activity from "@lucide/svelte/icons/activity";
  import DashboardCard from "./DashboardCard.svelte";
  import CardBody from "./CardBody.svelte";
  import CardState from "./CardState.svelte";
  import CardErrorState from "./CardErrorState.svelte";
  import {
    formatAuditType,
    formatDate,
    formatNumber,
    formatRelativeTime,
  } from "../../utils/format";
  import type { DashboardActivity } from "../../api/types/dashboard";
  import type { DashboardCardProps } from "./types";

  let {
    data,
    error,
    loading,
    busy,
    onRetry,
  }: DashboardCardProps<DashboardActivity> = $props();

  let events = $derived(data?.events ?? []);
  let total = $derived(data?.total ?? 0);
</script>

<DashboardCard title="Recent tag activity">
  {#snippet icon()}<Activity size={16} strokeWidth={1.75} />{/snippet}

  <CardBody {loading} {busy} hasError={!!error} empty={events.length === 0}>
    {#snippet errorState()}
      <CardErrorState title="Couldn’t load tag activity" {error} {onRetry} />
    {/snippet}

    {#snippet emptyState()}
      <CardState
        tone="neutral"
        title="No tag activity yet"
        description="Upstream tag additions, removals, and available upgrades show up here."
      >
        {#snippet icon()}<Activity size={30} strokeWidth={1.5} />{/snippet}
      </CardState>
    {/snippet}

    <ul class="item-list">
      {#each events as event (event.id)}
        <li>
          <button
            type="button"
            class="item-row"
            onclick={() => push(`/images/${event.imageId}`)}
          >
            <span class="item-main">
              <span class="item-title">{event.imageName}</span>
              <span
                class="item-sub muted"
                title="{formatAuditType(event.type)}: {event.data.tags.join(
                  ', ',
                )}"
              >
                {formatAuditType(event.type)}: {event.data.tags.join(", ")}
              </span>
            </span>
            <span class="item-time muted" title={formatDate(event.createdAt)}>
              {formatRelativeTime(event.createdAt)}
            </span>
          </button>
        </li>
      {/each}
    </ul>

    {#if total > events.length}
      <a class="view-all link" href="/tag-activity" use:link>
        View all {formatNumber(total)} tag events
      </a>
    {/if}
  </CardBody>
</DashboardCard>

<style>
  .item-time {
    flex-shrink: 0;
    font-size: 0.8125rem;
  }

</style>
