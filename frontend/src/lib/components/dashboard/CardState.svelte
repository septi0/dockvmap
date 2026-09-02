<script lang="ts">
  import type { Snippet } from "svelte";

  let {
    tone = "neutral",
    title,
    description,
    icon,
    action,
  }: {
    tone?: "neutral" | "success" | "error";
    title: string;
    description?: string;
    icon: Snippet;
    action?: Snippet;
  } = $props();
</script>

<div class="card-state">
  <span class="card-state-icon tone-{tone}">{@render icon()}</span>
  <p class="card-state-title">{title}</p>
  {#if description}
    <p class="card-state-desc muted">{description}</p>
  {/if}
  {#if action}
    <div class="card-state-action">{@render action()}</div>
  {/if}
</div>

<style>
  .card-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    text-align: center;
    gap: var(--space-2);
    padding: var(--space-4);
  }

  .card-state-icon {
    display: inline-flex;
    color: var(--color-text-faint);
  }

  .card-state-icon.tone-success {
    color: var(--color-success);
  }

  .card-state-icon.tone-error {
    color: var(--color-danger);
  }

  .card-state-title {
    margin: 0;
    font-size: 0.9375rem;
    font-weight: 500;
  }

  .card-state-desc {
    margin: 0;
    max-width: 36ch;
    font-size: 0.8125rem;
  }

  .card-state-action {
    margin-top: var(--space-2);
  }
</style>
