<script lang="ts">
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Modal from "./Modal.svelte";
  import Field from "./Field.svelte";
  import Button from "./Button.svelte";
  import RegistryOptionsFields from "./RegistryOptionsFields.svelte";
  import { updateRegistry } from "../api/registries";
  import { errorMessage } from "../api/client";
  import type { Registry } from "../api/types/registries";

  let {
    open,
    registry,
    onClose,
    onUpdated,
  }: {
    open: boolean;
    registry: Registry | null;
    onClose: () => void;
    onUpdated: (registry: Registry) => void;
  } = $props();

  let registryHost = $state("");
  let username = $state("");
  let credential = $state("");
  let insecure = $state(false);
  let allowSelfSignedCerts = $state(false);
  let changeAuth = $state(false);
  let error = $state<string | null>(null);
  let submitting = $state(false);

  $effect(() => {
    if (!registry) return;

    registryHost = registry.registry;
    username = registry.username ?? "";
    credential = "";
    insecure = registry.options.insecure;
    allowSelfSignedCerts = registry.options.allow_self_signed_certs;
    changeAuth = false;
    error = null;
  });

  function handleClose() {
    error = null;
    onClose();
  }

  function cancelAuthChange() {
    changeAuth = false;
    username = registry?.username ?? "";
    credential = "";
  }

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();

    if (!registry) return;

    error = null;
    submitting = true;

    try {
      const updated = await updateRegistry(registry.id, {
        registry: registryHost.trim(),
        username: changeAuth ? username.trim() : undefined,
        credential: changeAuth ? credential : undefined,
        options: { insecure, allow_self_signed_certs: allowSelfSignedCerts },
      });
      onUpdated(updated);
      onClose();
    } catch (err) {
      error = errorMessage(err, "Failed to update registry");
    } finally {
      submitting = false;
    }
  }
</script>

<Modal
  open={open && registry !== null}
  onClose={handleClose}
  title="Edit registry"
>
  <form onsubmit={handleSubmit}>
    {#if error}
      <p class="error">
        <TriangleAlert size={16} strokeWidth={2} />
        {error}
      </p>
    {/if}

    <Field label="Registry host" bind:value={registryHost} required />

    {#if !changeAuth}
      <div class="auth-status">
        <span class="muted">
          {registry?.authenticationConfigured
            ? "Authentication is configured."
            : "No authentication configured."}
        </span>
        <button type="button" class="link" onclick={() => (changeAuth = true)}
          >Change</button
        >
      </div>
    {:else}
      <Field label="Username" bind:value={username} autocomplete="username" />
      <Field
        label="Credential"
        type="password"
        bind:value={credential}
        autocomplete="new-password"
        placeholder="Leave both fields blank to remove authentication"
      />
      <button type="button" class="link" onclick={cancelAuthChange}>
        Cancel authentication change
      </button>
    {/if}

    <RegistryOptionsFields bind:insecure bind:allowSelfSignedCerts />

    <div class="actions">
      <Button type="button" variant="secondary" onclick={handleClose}
        >Cancel</Button
      >
      <Button type="submit" disabled={submitting}>
        {submitting ? "Saving…" : "Save changes"}
      </Button>
    </div>
  </form>
</Modal>

<style>
  .auth-status {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    font-size: 0.875rem;
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }

  .link {
    align-self: flex-start;
  }
</style>
