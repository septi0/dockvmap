<script lang="ts">
  import type { Snippet } from "svelte";

  let { label, children }: { label: string; children: Snippet } = $props();
</script>

<div class="detail-row">
  <span class="detail-label">{label}</span>
  <div class="detail-value">{@render children()}</div>
</div>

<style>
  .detail-row {
    display: grid;
    grid-template-columns: 140px 1fr;
    gap: var(--space-3);
    padding: var(--space-2) 0;
    border-bottom: 1px solid var(--color-border);
  }

  .detail-row:last-child {
    border-bottom: none;
  }

  .detail-label {
    font-size: 0.8125rem;
    font-weight: 500;
    color: var(--color-text-muted);
  }

  .detail-value {
    font-size: 0.875rem;
    word-break: break-word;
  }
</style>
