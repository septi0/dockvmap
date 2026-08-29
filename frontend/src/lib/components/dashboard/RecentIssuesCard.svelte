<script lang="ts">
  import { onMount } from "svelte";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import AsyncState from "../AsyncState.svelte";
  import { listRecentFailures } from "../../api/failures";
  import { ApiError } from "../../api/client";
  import type { RecentFailure } from "../../api/types/failures";
  import { formatDate } from "../../utils/format";

  let failures = $state<RecentFailure[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  async function load() {
    loading = true;
    error = null;

    try {
      failures = await listRecentFailures();
    } catch (err) {
      error =
        err instanceof ApiError ? err.message : "Failed to load recent issues";
    } finally {
      loading = false;
    }
  }

  onMount(load);
</script>

<div class="card section-card issues-card">
  <div class="card-head">
    <div class="card-head-title">
      <TriangleAlert size={16} strokeWidth={1.75} />
      <h2>Recent issues</h2>
    </div>
  </div>

  <AsyncState
    {loading}
    {error}
    empty={failures.length === 0}
    emptyMessage="No recent issues."
  >
    <div class="table-scroll">
      <table class="table">
        <thead>
          <tr>
            <th>Date</th>
            <th>Issue</th>
          </tr>
        </thead>
        <tbody>
          {#each failures as failure (failure.occurredAt + failure.message)}
            <tr>
              <td class="muted failure-date">{formatDate(failure.occurredAt)}</td>
              <td>{failure.message}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </AsyncState>
</div>

<style>
  .issues-card {
    grid-column: 1 / -1;
  }

  .table-scroll {
    overflow-x: auto;
  }

  .failure-date {
    white-space: nowrap;
  }
</style>
