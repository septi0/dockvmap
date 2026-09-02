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
      <h2>{title}</h2>
    </div>
    {#if action}{@render action()}{/if}
  </div>

  <div class="dashboard-card-body">
    {@render children()}
  </div>
</div>

<style>
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
