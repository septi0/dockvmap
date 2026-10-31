<script lang="ts">
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import DetailRow from "../DetailRow.svelte";
  import { formatDate, formatRelativeTime } from "../../utils/format";
  import type { Image } from "../../api/types/images";

  const SAME_PULL_MS = 60_000;

  let { image }: { image: Image } = $props();

  let checkedSincePull = $derived.by(() => {
    if (!image.lastRequestedAt) return false;
    if (!image.lastPulledAt) return true;

    return (
      new Date(image.lastRequestedAt).getTime() -
        new Date(image.lastPulledAt).getTime() >
      SAME_PULL_MS
    );
  });
</script>

<DetailRow label="Last pulled">
  <div class="detail-cell">
    {#if image.lastPulledAt}
      <span class="detail-cell-row">
        <span title={formatDate(image.lastPulledAt)}
          >{formatRelativeTime(image.lastPulledAt)}</span
        >
        <span class="pulled-tag">{image.lastPulledTag}</span>
      </span>
      {#if image.lastPulledTag !== image.tag}
        <p class="warning-text">
          <TriangleAlert size={14} strokeWidth={2} />
          Not pulled since the tag changed to {image.tag}.
        </p>
      {/if}
    {:else}
      <span class="muted">No pull recorded yet</span>
    {/if}
    {#if checkedSincePull && image.lastRequestedAt}
      <p class="pull-hint muted">
        A client last checked for a newer version
        <span title={formatDate(image.lastRequestedAt)}
          >{formatRelativeTime(image.lastRequestedAt)}</span
        >.
      </p>
    {/if}
  </div>
</DetailRow>

<style>
  .pulled-tag {
    font-family: monospace;
    color: var(--color-text-muted);
  }

  .pull-hint {
    margin: 0;
    font-size: 0.8125rem;
  }
</style>
