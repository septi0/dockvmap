<script lang="ts">
  import { push, link } from "svelte-spa-router";
  import CircleCheck from "@lucide/svelte/icons/circle-check";
  import HardDrive from "@lucide/svelte/icons/hard-drive";
  import DashboardCard from "./DashboardCard.svelte";
  import CardBody from "./CardBody.svelte";
  import CardState from "./CardState.svelte";
  import CardErrorState from "./CardErrorState.svelte";
  import ContainerVersion from "../hosts/ContainerVersion.svelte";
  import RedeployButton from "../hosts/RedeployButton.svelte";
  import { formatNumber } from "../../utils/format";
  import { redeployCommand } from "../../utils/hosts";
  import type { DashboardHosts } from "../../api/types/dashboard";
  import type { DashboardCardProps } from "./types";

  let { data, error, loading, busy, onRetry }: DashboardCardProps<DashboardHosts> = $props();

  let rows = $derived(
    (data?.behind ?? []).map((row) => ({ ...row, command: redeployCommand(row.container) })),
  );
  let hasCommands = $derived(rows.some((row) => row.command));
  let total = $derived(data?.outdated ?? 0);
</script>

<DashboardCard title="Behind on hosts">
  {#snippet icon()}<HardDrive size={16} strokeWidth={1.75} />{/snippet}

  <CardBody {loading} {busy} hasError={!!error} empty={rows.length === 0}>
    {#snippet errorState()}
      <CardErrorState title="Couldn’t load hosts" {error} {onRetry} />
    {/snippet}

    {#snippet emptyState()}
      <CardState
        tone="success"
        title="All containers current"
        description="Every container pulled through DockVMap runs the version it should."
      >
        {#snippet icon()}<CircleCheck size={30} strokeWidth={1.5} />{/snippet}
      </CardState>
    {/snippet}

    <ul class="item-list">
      {#each rows as { hostId, hostName, container, command } (`${hostId}:${container.id}`)}
        <li class="behind-row">
          <button type="button" class="item-row" onclick={() => push(`/hosts/${hostId}`)}>
            <span class="item-main">
              <span class="item-title container-name">{container.name}</span>
              <span class="behind-detail">
                <span class="muted">
                  {hostName}{container.composeProject ? ` · ${container.composeProject}` : ""}
                </span>
                {#if container.tracking}
                  <ContainerVersion tracking={container.tracking} />
                {/if}
              </span>
            </span>
          </button>
          {#if hasCommands}
            <span class="command-slot">
              {#if command}<RedeployButton {container} {command} />{/if}
            </span>
          {/if}
        </li>
      {/each}
    </ul>

    {#if total > rows.length}
      <a class="view-all link" href="/hosts" use:link>
        View all {formatNumber(total)} on Hosts
      </a>
    {/if}
  </CardBody>
</DashboardCard>

<style>
  .behind-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    border-bottom: 1px solid var(--color-border);
  }

  .behind-row:last-child {
    border-bottom: none;
  }

  .command-slot {
    display: inline-flex;
    flex-shrink: 0;
    width: calc(16px + 2 * var(--space-1));
  }

  .container-name {
    overflow-wrap: anywhere;
  }

  .behind-detail {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1) var(--space-2);
    font-size: 0.8125rem;
  }

  .behind-row .item-row {
    flex: 1;
    min-width: 0;
    border-bottom: none;
  }
</style>
