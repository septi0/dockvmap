<script lang="ts">
  import LoaderCircle from "@lucide/svelte/icons/loader-circle";

  let { text }: { text: string } = $props();
</script>

<span class="refreshing-indicator">
  <span class="icon spin"><LoaderCircle size={13} strokeWidth={2} /></span>
  {text}
</span>

<style>
  .refreshing-indicator {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    font-size: 0.75rem;
    color: var(--color-text-muted);
    white-space: nowrap;
  }

  .refreshing-indicator .icon {
    display: inline-flex;
  }
</style>
