<script lang="ts">
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Modal from "./Modal.svelte";
  import Field from "./Field.svelte";
  import Button from "./Button.svelte";
  import RegistryOptionsFields from "./RegistryOptionsFields.svelte";
  import {
    createRegistry,
    updateRegistry,
    testRegistry,
    testExistingRegistry,
  } from "../api/registries";
  import { errorMessage } from "../api/client";
  import type { Registry } from "../api/types/registries";

  let {
    open,
    mode,
    registry = null,
    onClose,
    onSaved,
  }: {
    open: boolean;
    mode: "create" | "edit";
    registry?: Registry | null;
    onClose: () => void;
    onSaved: (registry: Registry) => void;
  } = $props();

  let registryHost = $state("");
  let username = $state("");
  let credential = $state("");
  let editingAuth = $state(false);
  let insecure = $state(false);
  let allowSelfSignedCerts = $state(false);
  let error = $state<string | null>(null);
  let submitting = $state(false);
  let testing = $state(false);
  let testResult = $state<{ ok: boolean; message: string } | null>(null);

  const isEdit = $derived(mode === "edit");
  const ready = $derived(mode === "create" || registry !== null);

  $effect(() => {
    if (!open) return;

    registryHost = isEdit ? (registry?.registry ?? "") : "";
    username = isEdit ? (registry?.username ?? "") : "";
    credential = "";
    editingAuth = false;
    insecure = registry?.options.insecure ?? false;
    allowSelfSignedCerts = registry?.options.allow_self_signed_certs ?? false;
    error = null;
    testResult = null;
  });

  function baseParams() {
    return {
      registry: registryHost.trim(),
      options: { insecure, allow_self_signed_certs: allowSelfSignedCerts },
    };
  }

  async function handleTest() {
    testResult = null;
    testing = true;

    try {
      const result =
        isEdit && registry
          ? await testExistingRegistry(registry.id, {
              ...baseParams(),
              username: editingAuth ? username.trim() : undefined,
              credential: editingAuth ? credential : undefined,
            })
          : await testRegistry({
              ...baseParams(),
              username: editingAuth ? username.trim() : "",
              credential: editingAuth ? credential : "",
            });

      testResult = result.ok
        ? { ok: true, message: "Connection succeeded." }
        : { ok: false, message: result.error ?? "Connection failed." };
    } catch (err) {
      testResult = {
        ok: false,
        message: errorMessage(err, "Connection test failed"),
      };
    } finally {
      testing = false;
    }
  }

  function handleClose() {
    error = null;
    onClose();
  }

  function cancelAuthEdit() {
    editingAuth = false;
    username = isEdit ? (registry?.username ?? "") : "";
    credential = "";
  }

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    error = null;
    submitting = true;

    try {
      const saved =
        isEdit && registry
          ? await updateRegistry(registry.id, {
              ...baseParams(),
              username: editingAuth ? username.trim() : undefined,
              credential: editingAuth ? credential : undefined,
            })
          : await createRegistry({
              ...baseParams(),
              username: editingAuth ? username.trim() : "",
              credential: editingAuth ? credential : "",
            });

      onSaved(saved);
      onClose();
    } catch (err) {
      error = errorMessage(
        err,
        isEdit ? "Failed to update registry" : "Failed to create registry",
      );
    } finally {
      submitting = false;
    }
  }
</script>

<Modal
  open={open && ready}
  onClose={handleClose}
  title={isEdit ? "Edit registry" : "Add registry"}
>
  <form onsubmit={handleSubmit}>
    {#if error}
      <p class="error">
        <TriangleAlert size={16} strokeWidth={2} />
        {error}
      </p>
    {/if}

    <Field
      label="Registry host"
      bind:value={registryHost}
      placeholder={isEdit ? undefined : "docker.io"}
      required
    />

    {#if !editingAuth}
      <div class="auth-status">
        <span class="muted">
          {#if !isEdit}
            Anonymous - no authentication
          {:else if registry?.authenticationConfigured}
            Authentication is configured.
          {:else}
            No authentication configured.
          {/if}
        </span>
        <button type="button" class="link" onclick={() => (editingAuth = true)}>
          {isEdit ? "Change" : "Add authentication"}
        </button>
      </div>
    {:else}
      <Field
        label="Username"
        bind:value={username}
        autocomplete="username"
        required={!isEdit}
      />
      <Field
        label="Credential"
        type="password"
        bind:value={credential}
        autocomplete="new-password"
        required={!isEdit}
        placeholder={isEdit
          ? "Leave both fields blank to remove authentication"
          : undefined}
      />
      <button type="button" class="link" onclick={cancelAuthEdit}>
        {isEdit ? "Cancel authentication change" : "Remove authentication"}
      </button>
    {/if}

    <RegistryOptionsFields bind:insecure bind:allowSelfSignedCerts />

    {#if testResult}
      <p class="test-result" class:ok={testResult.ok} class:bad={!testResult.ok}>
        {testResult.message}
      </p>
    {/if}

    <div class="actions">
      <Button
        type="button"
        variant="secondary"
        disabled={testing || submitting || registryHost.trim() === ""}
        onclick={handleTest}
      >
        {testing ? "Testing…" : "Test connection"}
      </Button>
      <div class="actions-right">
        <Button type="button" variant="secondary" onclick={handleClose}>
          Cancel
        </Button>
        <Button type="submit" disabled={submitting}>
          {#if isEdit}
            {submitting ? "Saving…" : "Save changes"}
          {:else}
            {submitting ? "Adding…" : "Add registry"}
          {/if}
        </Button>
      </div>
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
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
  }

  .actions-right {
    display: flex;
    gap: var(--space-2);
  }

  .test-result {
    margin: var(--space-2) 0 0;
    font-size: 0.8125rem;
  }

  .test-result.ok {
    color: var(--color-success);
  }

  .test-result.bad {
    color: var(--color-danger);
  }

  .link {
    align-self: flex-start;
  }
</style>
