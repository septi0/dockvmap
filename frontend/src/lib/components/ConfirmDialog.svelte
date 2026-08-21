<script lang="ts">
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Modal from "./Modal.svelte";
  import Button from "./Button.svelte";

  let {
    open,
    title,
    message,
    confirmLabel = "Confirm",
    danger = false,
    error = null,
    submitting = false,
    onConfirm,
    onCancel,
  }: {
    open: boolean;
    title: string;
    message: string;
    confirmLabel?: string;
    danger?: boolean;
    error?: string | null;
    submitting?: boolean;
    onConfirm: () => void;
    onCancel: () => void;
  } = $props();

  function guardedCancel() {
    if (submitting) return;
    onCancel();
  }
</script>

<Modal {open} onClose={guardedCancel} {title}>
  {#if error}
    <p class="error">
      <TriangleAlert size={16} strokeWidth={2} />
      {error}
    </p>
  {/if}

  <p class="message">{message}</p>

  <div class="actions">
    <Button variant="secondary" onclick={guardedCancel} disabled={submitting}
      >Cancel</Button
    >
    <Button
      variant={danger ? "danger" : "primary"}
      onclick={onConfirm}
      disabled={submitting}
    >
      {submitting ? "Working…" : confirmLabel}
    </Button>
  </div>
</Modal>

<style>
  .message {
    margin: 0 0 var(--space-4);
    color: var(--color-text-muted);
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }
</style>
