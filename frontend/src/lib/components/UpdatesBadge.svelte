<script lang="ts">
  import { onMount } from 'svelte'
  import { link } from 'svelte-spa-router'
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert'
  import Check from '@lucide/svelte/icons/check'
  import { updatesCount } from '../services/updatesCount'

  onMount(() => updatesCount.start())

  let count = $derived($updatesCount)
</script>

<a
  href="/images?status=updateAvailable"
  use:link
  class="updates"
  class:updates-warning={count !== null && count > 0}
  class:updates-unknown={count === null}
>
  {#if count === null}
    <span class="icon"><TriangleAlert size={14} strokeWidth={1.75} /></span>
    <span class="label">Updates unknown</span>
  {:else if count > 0}
    <span class="icon"><TriangleAlert size={14} strokeWidth={1.75} /></span>
    <span class="value">{count}</span>
    <span class="label">{count === 1 ? 'update available' : 'updates available'}</span>
  {:else}
    <span class="icon"><Check size={14} strokeWidth={1.75} /></span>
    <span class="label">All caught up</span>
  {/if}
</a>

<style>
  .updates {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    padding: var(--space-1) var(--space-3);
    border-radius: var(--radius-full);
    background: var(--color-bg);
    border: 1px solid var(--color-border);
    text-decoration: none;
    transition:
      background-color var(--transition-fast),
      border-color var(--transition-fast);
  }

  .updates:hover {
    background: var(--color-surface-hover);
  }

  .icon {
    display: inline-flex;
    color: var(--color-text-faint);
    flex-shrink: 0;
  }

  .value {
    font-size: 0.8125rem;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
    color: var(--color-text);
  }

  .label {
    font-size: 0.75rem;
    color: var(--color-text-muted);
  }

  .updates-unknown .label {
    color: var(--color-text-faint);
  }

  .updates-warning {
    border-color: var(--color-warning);
  }

  .updates-warning .icon {
    color: var(--color-warning);
  }

  .updates-warning .label {
    color: var(--color-text);
  }
</style>
