<script lang="ts">
  import type { Snippet } from "svelte";

  let {
    title,
    count,
    icon,
    action,
    children,
  }: {
    title: string;
    count?: number;
    icon?: Snippet;
    action?: Snippet;
    children: Snippet;
  } = $props();
</script>

<div class="card section-card dashboard-card">
  <div class="card-head">
    <div class="card-head-title">
      {#if icon}{@render icon()}{/if}
      <h3>{title}{#if count}<span class="card-count">{count}</span>{/if}</h3>
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

  .card-count {
    margin-left: var(--space-2);
    font-size: 0.875rem;
    font-weight: 500;
    color: var(--color-text-muted);
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
