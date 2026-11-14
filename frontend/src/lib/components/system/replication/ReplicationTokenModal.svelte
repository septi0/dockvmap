<script lang="ts">
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Modal from "../../Modal.svelte";
  import Field from "../../Field.svelte";
  import Button from "../../Button.svelte";
  import CopyButton from "../../CopyButton.svelte";
  import { createReplicationToken } from "../../../api/replication";
  import { errorMessage } from "../../../api/client";
  import { followerConfig } from "../../../utils/replication";

  let {
    open,
    listen,
    onClose,
    onCreated,
  }: {
    open: boolean;
    listen: string;
    onClose: () => void;
    onCreated: () => void;
  } = $props();

  let label = $state("");
  let error = $state<string | null>(null);
  let submitting = $state(false);
  let config = $state<string | null>(null);

  function close() {
    label = "";
    error = null;
    config = null;
    onClose();
  }

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();

    if (!label.trim()) {
      error = "Label is required";
      return;
    }

    error = null;
    submitting = true;

    try {
      const result = await createReplicationToken(label.trim());
      config = followerConfig(listen, result.token);
      onCreated();
    } catch (err) {
      error = errorMessage(err, "Failed to create the replication token");
    } finally {
      submitting = false;
    }
  }
</script>

<Modal {open} onClose={close} title={config ? "Replication token created" : "Add replication token"} size={config ? "lg" : "md"}>
  {#if config}
    <p class="warning-text copy-warning">
      <TriangleAlert size={14} strokeWidth={2} />
      Copy it now; it won't be shown again.
    </p>
    <p class="section-hint muted">
      Add this to the follower's config, or set <code class="inline-code">DOCKVMAP_FOLLOWER_*</code>. Use this primary's
      own address, not a floating IP the follower may take over.
    </p>
    <div class="command-row">
      <code class="command">{config}</code>
      <CopyButton text={config} label="Copy follower config" copiedMessage="Follower config copied to clipboard." bordered />
    </div>
    <div class="modal-actions spaced">
      <Button onclick={close}>Done</Button>
    </div>
  {:else}
    <form onsubmit={handleSubmit}>
      {#if error}
        <p class="error">
          <TriangleAlert size={16} strokeWidth={2} />
          {error}
        </p>
      {/if}

      <Field label="Label" bind:value={label} placeholder="e.g. standby-1, mirror-office" required />

      <div class="modal-actions">
        <Button type="button" variant="secondary" onclick={close}>Cancel</Button>
        <Button type="submit" disabled={submitting}>{submitting ? "Creating…" : "Create token"}</Button>
      </div>
    </form>
  {/if}
</Modal>

<style>
  .copy-warning {
    margin-bottom: var(--space-3);
  }
</style>
