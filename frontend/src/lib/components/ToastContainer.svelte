<script lang="ts">
  import Check from "@lucide/svelte/icons/check";
  import X from "@lucide/svelte/icons/x";
  import { toast } from "../services/toast";
</script>

<div class="toasts">
  {#each $toast as item (item.id)}
    <div class="toast" role="status">
      <span class="icon"><Check size={16} strokeWidth={2} /></span>
      <span class="message">{item.message}</span>
      <button
        type="button"
        class="dismiss"
        onclick={() => toast.dismiss(item.id)}
        aria-label="Dismiss"
      >
        <X size={14} strokeWidth={2} />
      </button>
    </div>
  {/each}
</div>

<style>
  .toasts {
    position: fixed;
    right: var(--space-4);
    bottom: var(--space-4);
    z-index: 1000;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    pointer-events: none;
  }

  .toast {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 240px;
    max-width: 360px;
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-md);
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    box-shadow: var(--shadow-md);
    pointer-events: auto;
    animation: toast-in 0.15s ease-out;
  }

  .icon {
    display: inline-flex;
    flex-shrink: 0;
    color: var(--color-success);
  }

  .message {
    flex: 1;
    font-size: 0.875rem;
    color: var(--color-text);
  }

  .dismiss {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    padding: var(--space-1);
    border: none;
    background: transparent;
    border-radius: var(--radius-sm);
    color: var(--color-text-faint);
    cursor: pointer;
  }

  .dismiss:hover {
    background: var(--color-surface-hover);
    color: var(--color-text);
  }

  @keyframes toast-in {
    from {
      opacity: 0;
      transform: translateY(8px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
