<script lang="ts">
  import type { Snippet } from "svelte";
  import X from "@lucide/svelte/icons/x";

  let {
    open,
    onClose,
    title,
    size = "md",
    children,
  }: {
    open: boolean;
    onClose: () => void;
    title?: string;
    size?: "md" | "lg";
    children: Snippet;
  } = $props();

  let modalRef: HTMLDivElement | undefined = $state();

  const focusableSelector =
    'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';

  function trapFocus(event: KeyboardEvent) {
    if (!modalRef) return;

    const focusable = Array.from(
      modalRef.querySelectorAll<HTMLElement>(focusableSelector),
    );
    if (focusable.length === 0) {
      event.preventDefault();
      return;
    }

    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    const active = document.activeElement;

    if (event.shiftKey) {
      if (active === first || !modalRef.contains(active)) {
        event.preventDefault();
        last.focus();
      }
    } else {
      if (active === last || !modalRef.contains(active)) {
        event.preventDefault();
        first.focus();
      }
    }
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === "Escape") {
      onClose();
      return;
    }

    if (event.key === "Tab") trapFocus(event);
  }

  function handleOverlayClick(event: MouseEvent) {
    if (event.target === event.currentTarget) onClose();
  }

  $effect(() => {
    if (!open) return;

    const original = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    modalRef?.focus();

    return () => {
      document.body.style.overflow = original;
    };
  });
</script>

<svelte:window onkeydown={open ? handleKeydown : undefined} />

{#if open}
  <div class="overlay" onclick={handleOverlayClick} role="presentation">
    <div
      class="modal"
      class:lg={size === "lg"}
      role="dialog"
      aria-modal="true"
      aria-label={title}
      tabindex="-1"
      bind:this={modalRef}
    >
      <header class="modal-header">
        {#if title}<h2>{title}</h2>{/if}
        <button
          type="button"
          class="close"
          onclick={onClose}
          aria-label="Close"
        >
          <span class="icon"><X size={18} strokeWidth={1.75} /></span>
        </button>
      </header>

      <div class="modal-body">
        {@render children()}
      </div>
    </div>
  </div>
{/if}

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-4);
    z-index: 100;
  }

  .modal {
    width: 100%;
    max-width: 560px;
    max-height: 85vh;
    display: flex;
    flex-direction: column;
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-md);
  }

  .modal.lg {
    max-width: 760px;
    max-height: 90vh;
  }

  .modal-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    padding: var(--space-4) var(--space-5);
    border-bottom: 1px solid var(--color-border);
  }

  .modal-header h2 {
    margin: 0;
    font-size: 1.0625rem;
  }

  .close {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-1);
    border: none;
    background: transparent;
    border-radius: var(--radius-sm);
    cursor: pointer;
    color: var(--color-text-muted);
  }

  .close:hover {
    background: var(--color-surface-hover);
    color: var(--color-text);
  }

  .icon {
    display: inline-flex;
  }

  .modal-body {
    padding: var(--space-5);
    overflow-y: auto;
  }
</style>
