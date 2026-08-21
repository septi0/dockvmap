<script lang="ts">
  import KeyRound from "@lucide/svelte/icons/key-round";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Check from "@lucide/svelte/icons/check";
  import Modal from "./Modal.svelte";
  import Field from "./Field.svelte";
  import Button from "./Button.svelte";
  import { updatePassword } from "../api/users";
  import { ApiError } from "../api/client";

  let { open, onClose }: { open: boolean; onClose: () => void } = $props();

  let currentPassword = $state("");
  let newPassword = $state("");
  let confirmPassword = $state("");
  let error = $state<string | null>(null);
  let success = $state(false);
  let submitting = $state(false);

  $effect(() => {
    if (!open) return;

    currentPassword = "";
    newPassword = "";
    confirmPassword = "";
    error = null;
    success = false;
  });

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    error = null;
    success = false;

    if (newPassword !== confirmPassword) {
      error = "New passwords do not match";
      return;
    }

    submitting = true;

    try {
      await updatePassword(currentPassword, newPassword);
      success = true;
      currentPassword = "";
      newPassword = "";
      confirmPassword = "";
    } catch (err) {
      error =
        err instanceof ApiError ? err.message : "Failed to update password";
    } finally {
      submitting = false;
    }
  }
</script>

<Modal {open} {onClose} title="Change password">
  <p class="hint muted">
    Changing your password will sign you out of any other active sessions, on
    this device or elsewhere — the one you're using right now stays signed in.
  </p>

  <form onsubmit={handleSubmit}>
    {#if error}
      <p class="error">
        <TriangleAlert size={16} strokeWidth={2} />
        {error}
      </p>
    {/if}

    {#if success}
      <p class="success">
        <Check size={16} strokeWidth={2} />
        Password updated.
      </p>
    {/if}

    <Field
      label="Current password"
      type="password"
      bind:value={currentPassword}
      autocomplete="current-password"
      required
    />

    <div class="section-divider">
      <span>New password</span>
    </div>

    <Field
      label="New password"
      type="password"
      bind:value={newPassword}
      autocomplete="new-password"
      required
    />
    <Field
      label="Confirm new password"
      type="password"
      bind:value={confirmPassword}
      autocomplete="new-password"
      required
    />

    <div class="actions">
      <Button type="button" variant="secondary" onclick={onClose}>Close</Button>
      <Button type="submit" disabled={submitting}>
        <KeyRound size={16} strokeWidth={1.75} />
        {submitting ? "Updating…" : "Update password"}
      </Button>
    </div>
  </form>
</Modal>

<style>
  .hint {
    margin: 0 0 var(--space-4);
    font-size: 0.875rem;
  }

  form {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .section-divider {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin: var(--space-1) 0;
    color: var(--color-text-faint);
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .section-divider::after {
    content: "";
    flex: 1;
    height: 1px;
    background: var(--color-border);
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }
</style>
