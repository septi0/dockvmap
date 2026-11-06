<script lang="ts">
  import StateIndicator from "../StateIndicator.svelte";
  import type { Rollout } from "../../api/types/hosts";
  import { formatCount } from "../../utils/format";

  let {
    rollout,
    ofTotal = true,
    quiet = false,
  }: { rollout: Rollout; ofTotal?: boolean; quiet?: boolean } = $props();

  let behind = $derived(rollout.total - rollout.upToDate);
  let label = $derived(
    ofTotal && behind < rollout.total ? `${behind} of ${rollout.total} to redeploy` : `${behind} to redeploy`,
  );
  let title = $derived(
    `${behind} of ${rollout.total} running containers don't run the version you chose yet`,
  );
</script>

{#if behind > 0}
  {#if quiet}
    <span class="muted rollout" {title}>{label}</span>
  {:else}
    <StateIndicator tone="warning" {label} {title} />
  {/if}
{:else if ofTotal && !quiet}
  <span class="muted rollout">{formatCount(rollout.total, "container")}</span>
{/if}

<style>
  .rollout {
    white-space: nowrap;
  }
</style>
