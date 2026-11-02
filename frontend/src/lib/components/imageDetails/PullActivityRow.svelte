<script lang="ts">
  import DetailRow from "../DetailRow.svelte";
  import { formatDate, formatRelativeTime } from "../../utils/format";
  import type { Image } from "../../api/types/images";

  let { image }: { image: Image } = $props();
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
        <p class="pull-note muted">Not pulled since the tag changed to {image.tag}.</p>
      {/if}
    {:else}
      <span class="muted">No pull recorded yet</span>
    {/if}
  </div>
</DetailRow>

<style>
  .pull-note {
    margin: 0;
    font-size: 0.8125rem;
  }

  .pulled-tag {
    font-family: monospace;
    color: var(--color-text-muted);
  }
</style>
