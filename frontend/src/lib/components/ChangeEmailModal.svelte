<script lang="ts">
  import Mail from "@lucide/svelte/icons/mail";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Modal from "./Modal.svelte";
  import Field from "./Field.svelte";
  import Button from "./Button.svelte";
  import { updateEmail } from "../api/users";
  import { errorMessage } from "../api/client";

  let {
    open,
    currentEmail,
    onClose,
    onSaved,
  }: {
    open: boolean;
    currentEmail: string;
    onClose: () => void;
    onSaved: (email: string) => void;
  } = $props();

  let email = $state("");
  let submitting = $state(false);
  let error = $state<string | null>(null);

  const disabled = $derived(!email.trim() || email.trim() === currentEmail);

  $effect(() => {
    if (!open) return;

    email = currentEmail;
    error = null;
  });

  function guardedClose() {
    if (submitting) return;
    onClose();
  }

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    if (disabled) return;

    error = null;
    submitting = true;

    try {
      const trimmed = email.trim();
      await updateEmail(trimmed);
      onSaved(trimmed);
      onClose();
    } catch (err) {
      error = errorMessage(err, "Failed to update email");
    } finally {
      submitting = false;
    }
  }
</script>

<Modal {open} onClose={guardedClose} title="Change email">
  <p class="hint muted">
    This address receives tag alert emails and any account notifications.
  </p>

  <form onsubmit={handleSubmit}>
    {#if error}
      <p class="error">
        <TriangleAlert size={16} strokeWidth={2} />
        {error}
      </p>
    {/if}

    <Field
      label="Email"
      type="email"
      bind:value={email}
      autocomplete="email"
      disabled={submitting}
      required
    />

    <div class="actions">
      <Button
        type="button"
        variant="secondary"
        onclick={guardedClose}
        disabled={submitting}
      >
        Cancel
      </Button>
      <Button type="submit" disabled={disabled || submitting}>
        <Mail size={16} strokeWidth={1.75} />
        {submitting ? "Saving…" : "Save"}
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

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }
</style>
