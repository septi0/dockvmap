<script lang="ts">
  import { onMount } from "svelte";
  import { push } from "svelte-spa-router";
  import Activity from "@lucide/svelte/icons/activity";
  import AsyncState from "../AsyncState.svelte";
  import { listEvents } from "../../api/events";
  import { ApiError } from "../../api/client";
  import type { ImageEvent } from "../../api/types/events";
  import { formatDate, formatAuditType } from "../../utils/format";

  const LIMIT = 6;

  let events = $state<ImageEvent[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  async function load() {
    loading = true;
    error = null;

    try {
      const result = await listEvents(0);
      events = result.events.slice(0, LIMIT);
    } catch (err) {
      error =
        err instanceof ApiError ? err.message : "Failed to load recent activity";
    } finally {
      loading = false;
    }
  }

  onMount(load);
</script>

<div class="card section-card">
  <div class="card-head">
    <div class="card-head-title">
      <Activity size={16} strokeWidth={1.75} />
      <h2>Recent activity</h2>
    </div>
  </div>

  <AsyncState
    {loading}
    {error}
    empty={events.length === 0}
    emptyMessage="No tag activity recorded yet."
  >
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
            <span class="item-time muted">{formatDate(event.createdAt)}</span>
          </button>
        </li>
      {/each}
    </ul>
  </AsyncState>
</div>
