<script lang="ts">
  import { onMount } from "svelte";
  import RefreshCw from "@lucide/svelte/icons/refresh-cw";
  import AsyncState from "../AsyncState.svelte";
  import RefreshingIndicator from "../RefreshingIndicator.svelte";
  import { tagRefreshStatus } from "../../services/tagRefreshStatus";
  import { formatDate, formatRelativeTime } from "../../utils/format";

  onMount(() => tagRefreshStatus.watch());

  let status = $derived($tagRefreshStatus.data);
  let loading = $derived($tagRefreshStatus.loading);
  let error = $derived($tagRefreshStatus.data ? null : $tagRefreshStatus.error);
  let running = $derived(status?.running ?? false);

  let overdue = $derived(
    !running &&
      !!status?.nextDue &&
      new Date(status.nextDue).getTime() <= Date.now(),
  );

  let nextCheckLabel = $derived.by(() => {
    if (!status?.enabled) return "-";
    if (running) return "Running now";
    if (!status.nextDue) return "Pending";
    if (overdue) return "Due now";
    return formatRelativeTime(status.nextDue);
  });
</script>

<div class="card section-card">
  <div class="card-head">
    <div class="card-head-title">
      <RefreshCw size={16} strokeWidth={1.75} />
      <h2>Tag checks</h2>
    </div>

    {#if status}
      {#if running}
        <span class="badge badge-accent">Running</span>
      {:else if !status.enabled}
        <span class="badge">Disabled</span>
      {:else if overdue}
        <span class="badge badge-warning">Overdue</span>
      {:else}
        <span class="badge badge-accent">Active</span>
      {/if}
    {/if}
  </div>

  <AsyncState {loading} {error}>
    {#if running}
      <RefreshingIndicator text="Checking all images for tag changes…" />
    {:else if status?.enabled}
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
