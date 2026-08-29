<script lang="ts">
  import { onMount } from "svelte";
  import RefreshCw from "@lucide/svelte/icons/refresh-cw";
  import AsyncState from "../AsyncState.svelte";
  import { getTagRefreshStatus } from "../../api/worker";
  import { ApiError } from "../../api/client";
  import type { TagRefreshStatus } from "../../api/types/worker";
  import { formatDate, formatRelativeTime } from "../../utils/format";

  let status = $state<TagRefreshStatus | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  let overdue = $derived(
    !!status?.nextDue && new Date(status.nextDue).getTime() <= Date.now(),
  );

  let nextCheckLabel = $derived.by(() => {
    if (!status?.enabled) return "-";
    if (!status.nextDue) return "Pending";
    if (overdue) return "Due now";
    return formatRelativeTime(status.nextDue);
  });

  async function load() {
    loading = true;
    error = null;

    try {
      status = await getTagRefreshStatus();
    } catch (err) {
      error =
        err instanceof ApiError
          ? err.message
          : "Failed to load tag check status";
    } finally {
      loading = false;
    }
  }

  onMount(load);
</script>

<div class="card section-card">
  <div class="card-head">
    <div class="card-head-title">
      <RefreshCw size={16} strokeWidth={1.75} />
      <h2>Tag checks</h2>
    </div>

    {#if status}
      {#if !status.enabled}
        <span class="badge">Disabled</span>
      {:else if overdue}
        <span class="badge badge-warning">Overdue</span>
      {:else}
        <span class="badge badge-accent">Active</span>
      {/if}
    {/if}
  </div>

  <AsyncState {loading} {error}>
    {#if status?.enabled}
      <div class="stat-grid">
        <div class="stat-tile">
          <span
            class="stat-value"
            title={status.lastRun ? formatDate(status.lastRun) : undefined}
          >
            {status.lastRun ? formatRelativeTime(status.lastRun) : "Never"}
          </span>
          <span class="stat-label muted">Last check</span>
        </div>

        <div class="stat-tile">
          <span
            class="stat-value"
            class:danger={overdue}
            title={status.nextDue ? formatDate(status.nextDue) : undefined}
          >
            {nextCheckLabel}
          </span>
          <span class="stat-label muted">Next check</span>
        </div>
      </div>
    {:else if status}
      <p class="disabled-note muted">
        Automatic upstream tag checks are off. Set <code>tags_check_interval</code
        > to enable them.
      </p>
    {/if}
  </AsyncState>
</div>

<style>
  .disabled-note {
    margin: 0;
    font-size: 0.875rem;
  }
</style>
