<script lang="ts">
  import { link } from "svelte-spa-router";
  import CircleCheck from "@lucide/svelte/icons/circle-check";
  import RotateCw from "@lucide/svelte/icons/rotate-cw";
  import DashboardCard from "./DashboardCard.svelte";
  import CardBody from "./CardBody.svelte";
  import CardState from "./CardState.svelte";
  import CardErrorState from "./CardErrorState.svelte";
  import RedeployGroups from "../hosts/RedeployGroups.svelte";
  import { formatCount } from "../../utils/format";
  import { redeployGroups } from "../../utils/hosts";
  import type { DashboardHosts } from "../../api/types/dashboard";
  import type { DashboardCardProps } from "./types";

  const SHOWN = 4;

  let { data, error, loading, busy, onRetry }: DashboardCardProps<DashboardHosts> = $props();

  let groups = $derived(data ? redeployGroups(data.behind) : []);
  let shown = $derived(groups.slice(0, SHOWN));
</script>

<DashboardCard title="To redeploy" count={data?.outdated}>
  {#snippet icon()}<RotateCw size={16} strokeWidth={1.75} />{/snippet}

  <CardBody {loading} {busy} hasError={!!error} empty={groups.length === 0}>
    {#snippet errorState()}
      <CardErrorState title="Couldn’t load hosts" {error} {onRetry} />
    {/snippet}

    {#snippet emptyState()}
      <CardState
        tone="success"
        title="Every container runs the version you chose"
        description={data
          ? `${formatCount(data.trackedContainers, "container")} on ${formatCount(data.trackedHosts, "host")}.`
          : undefined}
      >
        {#snippet icon()}<CircleCheck size={30} strokeWidth={1.5} />{/snippet}
      </CardState>
    {/snippet}

    <RedeployGroups groups={shown} showHost />
    <p class="muted hint">Run each command on its host; this list clears by itself.</p>

    {#if groups.length > shown.length}
      <a class="view-all link" href="/hosts" use:link>View {groups.length - shown.length} more on Hosts</a>
    {/if}
  </CardBody>
</DashboardCard>

<style>
  .hint {
    margin: var(--space-3) 0 0;
    font-size: 0.8125rem;
  }
</style>
