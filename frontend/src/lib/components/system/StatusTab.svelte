<script lang="ts">
  import { onMount } from "svelte";
  import CircleCheck from "@lucide/svelte/icons/circle-check";
  import CircleX from "@lucide/svelte/icons/circle-x";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import AsyncState from "../AsyncState.svelte";
  import DetailRow from "../DetailRow.svelte";
  import { getSystemStatus } from "../../api/system";
  import { errorMessage } from "../../api/client";
  import type { SystemStatus } from "../../api/types/system";
  import {
    formatBytes,
    formatDate,
    formatRelativeTime,
  } from "../../utils/format";

  let status = $state<SystemStatus | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  onMount(load);

  async function load() {
    loading = true;
    error = null;

    try {
      status = await getSystemStatus();
    } catch (err) {
      error = errorMessage(err, "Failed to load system status");
    } finally {
      loading = false;
    }
  }
</script>

<AsyncState {loading} {error}>
  {#if status}
    <div class="card section-card">
      <h2>Build &amp; runtime</h2>
      <DetailRow label="Version">{status.version}</DetailRow>
      <DetailRow label="Database">
        {#if status.database.reachable}
          <span class="state ok">
            <CircleCheck size={14} strokeWidth={2} /> Reachable
          </span>
        {:else}
          <span class="state bad">
            <CircleX size={14} strokeWidth={2} /> Unreachable
          </span>
        {/if}
      </DetailRow>
      <DetailRow label="Schema version">{status.database.schemaVersion}</DetailRow>
      <DetailRow label="Database size">
        {formatBytes(status.database.sizeBytes)}
      </DetailRow>
      <DetailRow label="Started">
        {formatDate(status.startedAt)} ({formatRelativeTime(status.startedAt)})
      </DetailRow>
      <DetailRow label="Data path">
        <code class="mono">{status.dataPath}</code>
      </DetailRow>
      <DetailRow label="Database file">
        <code class="mono">{status.database.path}</code>
      </DetailRow>
    </div>

    <div class="card section-card">
      <h2>Startup warnings</h2>
      {#if status.configWarnings.length === 0}
        <p class="muted">None.</p>
      {:else}
        <ul class="warnings">
          {#each status.configWarnings as warning, i (i)}
            <li>
              <TriangleAlert size={14} strokeWidth={2} />
              <span>{warning}</span>
            </li>
          {/each}
        </ul>
      {/if}
    </div>

    <div class="card section-card">
      <h2>Effective settings</h2>
      <DetailRow label="Virtual tag">
        <code class="mono">{status.virtualTag}</code>
      </DetailRow>
      <DetailRow label="Trusted proxies">
        {#if status.trustedProxies.length === 0}
          <span class="muted">None (client IP is the immediate TCP peer)</span>
        {:else}
          <span class="mono">{status.trustedProxies.join(", ")}</span>
        {/if}
      </DetailRow>
      <DetailRow label="Proxy authentication">
        {status.proxyAuthEnabled ? "Enabled" : "Disabled"}
      </DetailRow>
    </div>
  {/if}
</AsyncState>

<style>
  .state {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
  }

  .state.ok {
    color: var(--color-success);
  }

  .state.bad {
    color: var(--color-danger);
  }

  .mono {
    font-family: ui-monospace, monospace;
    font-size: 0.8125rem;
  }

  .warnings {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .warnings li {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    font-size: 0.8125rem;
    color: var(--color-warning);
  }

  .warnings li :global(svg) {
    flex-shrink: 0;
    margin-top: 2px;
  }
</style>
