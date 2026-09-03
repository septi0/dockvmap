<script lang="ts">
  let {
    rows = 4,
    bordered = true,
  }: {
    rows?: number;
    bordered?: boolean;
  } = $props();
</script>

<div class="list-skeleton" class:bordered>
  {#each Array.from({ length: rows }) as _, i (i)}
    <div class="row">
      <span class="skeleton-bar line-title"></span>
      <span class="skeleton-bar line-sub"></span>
    </div>
  {/each}
</div>

<style>
  .list-skeleton {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .list-skeleton:not(.bordered) {
    gap: 0;
  }

  .row {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .list-skeleton.bordered .row {
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
  }

  .list-skeleton:not(.bordered) .row {
    padding: var(--space-3) 0;
    border-bottom: 1px solid var(--color-border);
  }

  .list-skeleton:not(.bordered) .row:last-child {
    border-bottom: none;
  }

  .line-title {
    width: 38%;
    height: 0.875rem;
  }

  .line-sub {
    width: 62%;
  }
</style>
