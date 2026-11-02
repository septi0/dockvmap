<script lang="ts">
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Modal from "../Modal.svelte";
  import Field from "../Field.svelte";
  import Button from "../Button.svelte";
  import { createHost } from "../../api/hosts";
  import { errorMessage } from "../../api/client";
  import type { HostToken } from "../../api/types/hosts";

  let {
    open,
    onClose,
    onCreated,
  }: {
    open: boolean;
    onClose: () => void;
    onCreated: (result: HostToken) => void;
  } = $props();

  let name = $state("");
  let error = $state<string | null>(null);
  let submitting = $state(false);

  function reset() {
    name = "";
    error = null;
    submitting = false;
  }

  function handleClose() {
    reset();
    onClose();
  }

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    error = null;

    const trimmedName = name.trim();

    if (!trimmedName) {
      error = "Name is required";
      return;
    }

    submitting = true;

    try {
      const result = await createHost({ name: trimmedName });
      onCreated(result);
      reset();
      onClose();
    } catch (err) {
      error = errorMessage(err, "Failed to add host");
    } finally {
      submitting = false;
    }
  }
</script>

<Modal {open} onClose={handleClose} title="Add host">
  <form onsubmit={handleSubmit}>
    {#if error}
      <p class="error">
        <TriangleAlert size={16} strokeWidth={2} />
        {error}
      </p>
    {/if}

    <Field
      label="Name"
      bind:value={name}
      placeholder="e.g. server2, nas, build-runner"
      required
    />

    <p class="section-hint muted">
      Next you'll get a token and a ready-to-use compose file for the agent on
      this host.
    </p>

    <div class="modal-actions">
      <Button type="button" variant="secondary" onclick={handleClose}
        >Cancel</Button
      >
      <Button type="submit" disabled={submitting}>
        {submitting ? "Adding…" : "Add host"}
      </Button>
    </div>
  </form>
</Modal>
