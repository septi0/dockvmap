<script lang="ts">
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import { groupPullStatus } from "../utils/groupPulls";
  import type { ImageGroup } from "../api/types/imageGroups";

  let { group, showNames = false }: { group: ImageGroup; showNames?: boolean } =
    $props();

  let status = $derived(groupPullStatus(group));
</script>

{#if status}
  <p
    class="warning-text"
    title={`Not pulled on ${group.tag} yet: ${status.missing.join(", ")}`}
  >
    <TriangleAlert size={14} strokeWidth={2} />
    <span>
      {status.pulled} of {status.total} pulled on {group.tag}{#if showNames}. Not yet: {status.missing.join(", ")}{/if}
    </span>
  </p>
{/if}
