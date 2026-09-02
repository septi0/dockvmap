<script lang="ts">
  import { push } from "svelte-spa-router";
  import Activity from "@lucide/svelte/icons/activity";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import DashboardCard from "./DashboardCard.svelte";
  import CardBody from "./CardBody.svelte";
  import CardState from "./CardState.svelte";
  import { listEvents } from "../../api/events";
  import { ApiError } from "../../api/client";
  import { dashboardRefresh } from "../../services/dashboardRefresh";
  import type { ImageEvent } from "../../api/types/events";
  import {
    formatAuditType,
    formatDate,
    formatRelativeTime,
  } from "../../utils/format";

  const CARD_ID = "recent-tag-activity";
  const LIMIT = 8;

  let events = $state<ImageEvent[]>([]);
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
      const result = await listEvents(0);
      events = result.events.slice(0, LIMIT);
    } catch (err) {
      error =
        err instanceof ApiError ? err.message : "Failed to load tag activity";
    } finally {
      loading = false;
      loaded = true;
      dashboardRefresh.end(CARD_ID, !error);
    }
  }
</script>

<DashboardCard title="Recent tag activity">
  {#snippet icon()}<Activity size={16} strokeWidth={1.75} />{/snippet}

  <CardBody
    loading={loading && !loaded}
    busy={loading && loaded}
    hasError={!!error}
    empty={events.length === 0}
  >
    {#snippet errorState()}
      <CardState
        tone="error"
        title="Couldn’t load tag activity"
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
  </CardBody>
</DashboardCard>
