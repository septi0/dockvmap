<script lang="ts">
  import LoaderCircle from "@lucide/svelte/icons/loader-circle";

  let { active = false }: { active?: boolean } = $props();
</script>

{#if active}
  <span class="busy-overlay" aria-hidden="true">
    <LoaderCircle size={16} strokeWidth={3} />
  </span>
{/if}

<style>
  .busy-overlay {
    position: absolute;
    top: 0;
    right: 0;
    z-index: 2;
    display: inline-flex;
    color: var(--color-accent);
    animation: spin 0.8s linear infinite;
  }
</style>
