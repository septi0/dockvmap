<script lang="ts">
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Modal from "../Modal.svelte";
  import Field from "../Field.svelte";
  import Button from "../Button.svelte";
  import { createHost, updateHost } from "../../api/hosts";
  import { errorMessage } from "../../api/client";
  import type { Host, HostConnection } from "../../api/types/hosts";

  let {
    open,
    host = null,
    onClose,
    onSaved,
  }: {
    open: boolean;
    host?: Host | null;
    onClose: () => void;
    onSaved: (result: HostConnection) => void;
  } = $props();

  let name = $state("");
  let address = $state("");
  let error = $state<string | null>(null);
  let submitting = $state(false);

  $effect(() => {
    if (!open) return;

    name = host?.name ?? "";
    address = host?.address ?? "";
    error = null;
  });

  function handleClose() {
    if (submitting) return;
    onClose();
  }

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    error = null;

    const params = { name: name.trim(), address: address.trim() };

    if (!params.name || !params.address) {
      error = "Name and address are required";
      return;
    }

    submitting = true;

    try {
      let id: number;

      if (host) {
        await updateHost(host.id, params);
        id = host.id;
      } else {
        id = (await createHost(params)).id;
      }

      onSaved({ id, ...params });
      onClose();
    } catch (err) {
      error = errorMessage(err, host ? "Failed to save the host" : "Failed to add host");
    } finally {
      submitting = false;
    }
  }
</script>

<Modal {open} onClose={handleClose} title={host ? "Edit host" : "Add host"}>
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

    <Field
      label="Address"
      bind:value={address}
      placeholder="e.g. 10.0.20.15 or nas.lan:9002"
      required
    />

    <p class="section-hint muted">
      DockVMap connects to the agent at this address, on port 9002 unless you
      add another one.
      {#if !host}Next you'll get a ready-to-use compose file for the agent.{/if}
    </p>

    <div class="modal-actions">
      <Button type="button" variant="secondary" onclick={handleClose}
        >Cancel</Button
      >
      <Button type="submit" disabled={submitting}>
        {#if host}
          {submitting ? "Saving…" : "Save"}
        {:else}
          {submitting ? "Adding…" : "Add host"}
        {/if}
      </Button>
    </div>
  </form>
</Modal>
