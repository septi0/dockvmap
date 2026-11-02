<script lang="ts">
  import { link } from "svelte-spa-router";
  import ContainerState from "../hosts/ContainerState.svelte";
  import ContainerVersion from "../hosts/ContainerVersion.svelte";
  import HostStatus from "../hosts/HostStatus.svelte";
  import RolloutSummary from "../hosts/RolloutSummary.svelte";
  import { getImageHosts } from "../../api/hosts";
  import { errorMessage } from "../../api/client";
  import { formatCount } from "../../utils/format";
  import { createVisibilityPoller } from "../../utils/visibilityPoller";
  import type { Host, HostContainer, ImageContainers } from "../../api/types/hosts";

  const REFRESH_INTERVAL_MS = 15_000;

  let { imageId, refreshKey }: { imageId: number; refreshKey: string } =
    $props();

  let result = $state<ImageContainers | null>(null);
  let error = $state<string | null>(null);
  let loadToken = 0;

  let rows = $derived(
    (result?.hosts ?? [])
      .flatMap(({ host, containers }) =>
        (containers ?? []).map((container) => ({ host, container })),
      )
      .sort((a, b) => attentionRank(a.host, a.container) - attentionRank(b.host, b.container)),
  );

  function attentionRank(host: Host, container: HostContainer): number {
    if (!host.current) return 2;
    if (!container.tracking?.needsUpdate) return 3;

    return container.tracking.status === "outdated" ? 0 : 1;
  }

  async function load() {
    const requestId = ++loadToken;

    try {
      const fresh = await getImageHosts(imageId);
      if (requestId !== loadToken) return;
      result = fresh;
      error = null;
    } catch (err) {
      if (requestId !== loadToken || result) return;
      error = errorMessage(err, "Failed to load where this image runs");
    }
  }

  $effect(() => {
    imageId;
    refreshKey;

    return createVisibilityPoller({
      tick: load,
      intervalMs: REFRESH_INTERVAL_MS,
    }).start();
  });
</script>

{#snippet version(host: Host, container: HostContainer)}
  {#if !host.current}
    <span class="muted" title="This host can't be checked right now">Unknown</span>
  {:else if container.tracking}
    <ContainerVersion tracking={container.tracking} />
  {/if}
{/snippet}

{#if error || (result && result.configuredHosts > 0)}
  <div class="card section-card">
    <h2 class="heading-with-count">
      Running on
      {#if result && result.hosts.length > 0}
        <span class="muted heading-count">{formatCount(result.hosts.length, "host")}</span>
        {#if result.rollout.total > 0}
          <span class="heading-count"><RolloutSummary rollout={result.rollout} quiet /></span>
        {/if}
      {/if}
    </h2>

    {#if error}
      <p class="error">{error}</p>
    {:else if result && result.hosts.length === 0}
      <p class="section-hint muted">No online host runs this image.</p>
    {:else if result}
      <div class="table-wrap">
        <table class="table">
          <thead>
            <tr>
              <th class="phone-hidden">Host</th>
              <th>Container</th>
              <th>State</th>
              <th class="phone-hidden">Version</th>
            </tr>
          </thead>
          <tbody>
            {#each rows as { host, container } (`${host.id}:${container.id}`)}
              <tr>
                <td class="phone-hidden">
                  <a class="row-link host-name" href={`/hosts/${host.id}`} use:link>{host.name}</a>
                </td>
                <td>
                  {container.name}
                  <div class="phone-only row-detail">
                    <a class="row-link" href={`/hosts/${host.id}`} use:link>{host.name}</a>
                    {@render version(host, container)}
                  </div>
                </td>
                <td>
                  {#if host.current}
                    <ContainerState {container} />
                  {:else}
                    <HostStatus {host} showLastSeen inline />
                  {/if}
                </td>
                <td class="phone-hidden">{@render version(host, container)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
{/if}

<style>
  .row-detail {
    margin-top: var(--space-1);
  }
</style>
