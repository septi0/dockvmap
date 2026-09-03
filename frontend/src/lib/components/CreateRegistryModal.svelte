<script lang="ts">
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Modal from "./Modal.svelte";
  import Field from "./Field.svelte";
  import Button from "./Button.svelte";
  import RegistryOptionsFields from "./RegistryOptionsFields.svelte";
  import { createRegistry, testRegistry } from "../api/registries";
  import { errorMessage } from "../api/client";
  import type { Registry } from "../api/types/registries";

  let {
    open,
    onClose,
    onCreated,
  }: {
    open: boolean;
    onClose: () => void;
    onCreated: (registry: Registry) => void;
  } = $props();

  let registryHost = $state("");
  let username = $state("");
  let credential = $state("");
  let addAuth = $state(false);
  let insecure = $state(false);
  let allowSelfSignedCerts = $state(false);
  let error = $state<string | null>(null);
  let submitting = $state(false);
  let testing = $state(false);
  let testResult = $state<{ ok: boolean; message: string } | null>(null);

  function reset() {
    registryHost = "";
    username = "";
    credential = "";
    addAuth = false;
    insecure = false;
    allowSelfSignedCerts = false;
    error = null;
    submitting = false;
    testing = false;
    testResult = null;
  }

  function currentParams() {
    return {
      registry: registryHost.trim(),
      username: addAuth ? username.trim() : "",
      credential: addAuth ? credential : "",
      options: { insecure, allow_self_signed_certs: allowSelfSignedCerts },
    };
  }

  async function handleTest() {
    testResult = null;
    testing = true;

    try {
      const result = await testRegistry(currentParams());
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
    reset();
    onClose();
  }

  function removeAuth() {
    addAuth = false;
    username = "";
    credential = "";
  }

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    error = null;
    submitting = true;

    try {
      const registry = await createRegistry(currentParams());
      onCreated(registry);
      reset();
      onClose();
    } catch (err) {
      error = errorMessage(err, "Failed to create registry");
    } finally {
      submitting = false;
    }
  }
</script>

<Modal {open} onClose={handleClose} title="Add registry">
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
      placeholder="docker.io"
      required
    />

    {#if !addAuth}
      <div class="auth-status">
        <span class="muted">Anonymous - no authentication</span>
        <button type="button" class="link" onclick={() => (addAuth = true)}>
          Add authentication
        </button>
      </div>
    {:else}
      <Field
        label="Username"
        bind:value={username}
        autocomplete="username"
        required
      />
      <Field
        label="Credential"
        type="password"
        bind:value={credential}
        autocomplete="new-password"
        required
      />
      <button type="button" class="link" onclick={removeAuth}
        >Remove authentication</button
      >
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
        <Button type="button" variant="secondary" onclick={handleClose}
          >Cancel</Button
        >
        <Button type="submit" disabled={submitting}>
          {submitting ? "Adding…" : "Add registry"}
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
