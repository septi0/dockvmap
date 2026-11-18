<script lang="ts">
  import { onMount } from "svelte";
  import AsyncState from "../AsyncState.svelte";
  import PrimaryReplication from "./replication/PrimaryReplication.svelte";
  import { getReplicationStatus } from "../../api/replication";
  import { errorMessage } from "../../api/client";
  import { createVisibilityPoller } from "../../utils/visibilityPoller";
  import type { ReplicationStatus } from "../../api/types/replication";

  const POLL_INTERVAL_MS = 15_000;

  let status = $state<ReplicationStatus | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  async function load() {
    try {
      status = await getReplicationStatus();
      error = null;
    } catch (err) {
      error = errorMessage(err, "Failed to load replication status");
    } finally {
      loading = false;
    }
  }

  onMount(() => createVisibilityPoller({ tick: load, intervalMs: POLL_INTERVAL_MS }).start());
</script>

<AsyncState loading={loading && !status} {error}>
  {#if status?.primary}
    <PrimaryReplication status={status.primary} onChanged={load} />
  {:else if status}
    <div class="card section-card">
      <h2>Replication is off</h2>
      <p class="muted">
        Turn it on with <code class="inline-code">replication.enabled</code> in config to let followers copy this
        DockVMap, or set <code class="inline-code">follower.enabled</code> to make this one a read-only follower, which has no web UI.
      </p>
    </div>
  {/if}
</AsyncState>
