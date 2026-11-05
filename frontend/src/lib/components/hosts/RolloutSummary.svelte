<script lang="ts">
  import type { Rollout } from "../../api/types/hosts";
  import { formatCount } from "../../utils/format";

  let {
    rollout,
    compact = false,
    quiet = false,
  }: { rollout: Rollout; compact?: boolean; quiet?: boolean } = $props();

  let outdated = $derived(rollout.total - rollout.upToDate);
  let label = $derived.by(() => {
    if (outdated === 0) return compact ? "Current" : "All current";

    return compact ? `${outdated} behind` : `${formatCount(outdated, "container")} behind`;
  });
  let title = $derived(
    outdated > 0
      ? `${outdated} of ${rollout.total} running containers aren't on the current version yet`
      : `All ${rollout.total} running containers are on the current version`,
  );
</script>

{#if outdated === 0 || quiet}
  <span class="muted rollout" {title}>{label}</span>
{:else}
  <span class="badge badge-warning rollout" {title}>{label}</span>
{/if}

<style>
  .rollout {
    white-space: nowrap;
  }
</style>
