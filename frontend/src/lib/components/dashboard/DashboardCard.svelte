<script lang="ts">
  import type { Snippet } from "svelte";

  let {
    title,
    icon,
    action,
    children,
  }: {
    title: string;
    icon?: Snippet;
    action?: Snippet;
    children: Snippet;
  } = $props();
</script>

<div class="card section-card dashboard-card">
  <div class="card-head">
    <div class="card-head-title">
      {#if icon}{@render icon()}{/if}
      <h3>{title}</h3>
    </div>
    {#if action}{@render action()}{/if}
  </div>

  <div class="dashboard-card-body">
    {@render children()}
  </div>
</div>

<style>
  .card-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    margin-bottom: var(--space-4);
  }

  .card-head-title {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .card-head-title h3 {
    margin: 0;
    font-size: 1.0625rem;
  }

  .card-head-title :global(svg) {
    color: var(--color-text-muted);
  }

  .dashboard-card {
    display: flex;
    flex-direction: column;
    margin-bottom: 0;
  }

  .dashboard-card-body {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-height: 0;
  }
</style>
