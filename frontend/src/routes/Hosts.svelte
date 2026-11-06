<script lang="ts">
  import { onMount } from "svelte";
  import { link, push } from "svelte-spa-router";
  import Plus from "@lucide/svelte/icons/plus";
  import HardDrive from "@lucide/svelte/icons/hard-drive";
  import AppShell from "../lib/components/AppShell.svelte";
  import PageTitle from "../lib/components/PageTitle.svelte";
  import AsyncState from "../lib/components/AsyncState.svelte";
  import Button from "../lib/components/Button.svelte";
  import HostModal from "../lib/components/hosts/HostModal.svelte";
  import HostConnectModal from "../lib/components/hosts/HostConnectModal.svelte";
  import HostStatus from "../lib/components/hosts/HostStatus.svelte";
  import StateIndicator from "../lib/components/StateIndicator.svelte";
  import RolloutSummary from "../lib/components/hosts/RolloutSummary.svelte";
  import { listHosts } from "../lib/api/hosts";
  import { errorMessage } from "../lib/api/client";
  import { createVisibilityPoller } from "../lib/utils/visibilityPoller";
  import { formatCount, formatDate, formatRelativeTime } from "../lib/utils/format";
  import { proxyAddress } from "../lib/utils/pullInfo";
  import { getPullInfo } from "../lib/api/metrics";
  import type { Host, HostConnection } from "../lib/api/types/hosts";
  import type { PullInfo } from "../lib/api/types/metrics";

  const REFRESH_INTERVAL_MS = 30_000;

  let hosts = $state<Host[]>([]);
  let loading = $state(true);
  let loaded = $state(false);
  let error = $state<string | null>(null);
  let showAddModal = $state(false);
  let connection = $state<HostConnection | null>(null);
  let pullInfo = $state<PullInfo | null>(null);
  let loadToken = 0;

  async function load(quiet = false) {
    const requestId = ++loadToken;
    loading = !quiet || !loaded;
    error = null;

    try {
      const result = await listHosts();
      if (requestId !== loadToken) return;
      hosts = result;
    } catch (err) {
      if (requestId !== loadToken) return;
      error = errorMessage(err, "Failed to load hosts");
    } finally {
      if (requestId === loadToken) {
        loading = false;
        loaded = true;
      }
    }
  }

  let noProxyContainers = $derived(
    hosts.some((host) => host.current && host.runningContainers > 0) &&
      hosts.every((host) => host.proxyContainers === 0),
  );

  const poller = createVisibilityPoller({
    tick: () => load(true),
    intervalMs: REFRESH_INTERVAL_MS,
  });

  onMount(() => {
    getPullInfo()
      .then((info) => (pullInfo = info))
      .catch(() => {});

    return poller.start();
  });

  function openHost(event: MouseEvent, id: number) {
    if ((event.target as HTMLElement).closest("a, button")) return;
    push(`/hosts/${id}`);
  }

  function handleCreated(result: HostConnection) {
    connection = result;
    load();
  }

  function closeConnection() {
    connection = null;
    load();
  }
</script>

<PageTitle title="Hosts" />

<AppShell>
  <div class="list-header">
    <div class="title-row">
      <HardDrive size={20} strokeWidth={1.75} />
      <h1>Hosts</h1>
    </div>
    <Button onclick={() => (showAddModal = true)}>
      <Plus size={16} strokeWidth={2} />
      Add host
    </Button>
  </div>

  <AsyncState
    loading={loading && !loaded}
    busy={loading && loaded}
    {error}
    empty={hosts.length === 0}
    emptyMessage="No hosts yet. Add one and run the agent on it to see its containers and which virtual images they run."
    columns={4}
  >
    {#snippet emptyAction()}
      <Button onclick={() => (showAddModal = true)}>
        <Plus size={16} strokeWidth={2} />
        Add host
      </Button>
    {/snippet}

    <div class="card table-wrap">
      <table class="table">
        <thead>
          <tr>
            <th>Host</th>
            <th class="phone-hidden">Status</th>
            <th>Containers</th>
          </tr>
        </thead>
        <tbody>
          {#each hosts as host (host.id)}
            <tr class="clickable" onclick={(event) => openHost(event, host.id)}>
              <td>
                <a class="row-link host-name" href={`/hosts/${host.id}`} use:link
                  >{host.name}</a
                >
                <div class="phone-only host-status"><HostStatus {host} showLastSeen /></div>
              </td>
              <td class="phone-hidden"><HostStatus {host} showLastSeen /></td>
              <td>
                {#if host.lastSeenAt}
                  <div class:muted={!host.current}>
                    {host.runningContainers} running
                    <span class="muted stopped-count"
                      ><span class="phone-hidden">·</span> {host.stoppedContainers} stopped</span
                    >
                  </div>
                  <div class="cell-detail">
                    {#if !host.current}
                      <span class="muted">last known</span>
                    {:else if host.stacks > 0}
                      <span class="muted">{formatCount(host.stacks, "stack")}</span>
                    {/if}
                    {#if host.lastStartedAt}
                      <span class="muted" title={formatDate(host.lastStartedAt)}
                        >last restart {formatRelativeTime(host.lastStartedAt)}</span
                      >
                    {/if}
                  </div>
                  {#if host.crashedContainers > 0 || host.unhealthyContainers > 0 || (host.current && host.rollout.upToDate < host.rollout.total)}
                    <div class="host-issues">
                      {#if host.crashedContainers > 0}
                        <StateIndicator
                          tone="failed"
                          label="{host.crashedContainers} crashed"
                          title="Containers that crashed in the last 7 days or keep restarting"
                        />
                      {/if}
                      {#if host.unhealthyContainers > 0}
                        <StateIndicator tone="warning" label="{host.unhealthyContainers} unhealthy" />
                      {/if}
                      {#if host.current}
                        <RolloutSummary rollout={host.rollout} ofTotal={false} />
                      {/if}
                    </div>
                  {/if}
                {:else}
                  <span class="muted">-</span>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if pullInfo && noProxyContainers}
      <p class="muted proxy-hint">
        No containers were pulled through <code class="inline-code"
          >{proxyAddress(pullInfo)}</code
        >. If clients pull from another address, set
        <code class="inline-code">proxy_public_host</code> in config.
      </p>
    {/if}
  </AsyncState>
</AppShell>

<HostModal
  open={showAddModal}
  onClose={() => (showAddModal = false)}
  onSaved={handleCreated}
/>

<HostConnectModal {connection} onClose={closeConnection} />

<style>
  .cell-detail {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1) var(--space-2);
    margin-top: 2px;
    font-size: 0.8125rem;
  }

  .host-status {
    margin-top: var(--space-1);
  }

  .host-issues {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1) var(--space-3);
    margin-top: var(--space-1);
    font-size: 0.8125rem;
  }

  .proxy-hint {
    margin: var(--space-3) 0 0;
    font-size: 0.8125rem;
  }

  @media (max-width: 768px) {
    .stopped-count {
      display: block;
    }
  }
</style>
