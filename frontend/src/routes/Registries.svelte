<script lang="ts">
  import { onMount } from "svelte";
  import Plus from "@lucide/svelte/icons/plus";
  import Server from "@lucide/svelte/icons/server";
  import Pencil from "@lucide/svelte/icons/pencil";
  import Trash2 from "@lucide/svelte/icons/trash-2";
  import AppShell from "../lib/components/AppShell.svelte";
  import PageTitle from "../lib/components/PageTitle.svelte";
  import AsyncState from "../lib/components/AsyncState.svelte";
  import Button from "../lib/components/Button.svelte";
  import CreateRegistryModal from "../lib/components/CreateRegistryModal.svelte";
  import EditRegistryModal from "../lib/components/EditRegistryModal.svelte";
  import ConfirmDialog from "../lib/components/ConfirmDialog.svelte";
  import { listRegistries, deleteRegistry } from "../lib/api/registries";
  import { ApiError } from "../lib/api/client";
  import { toast } from "../lib/services/toast";
  import type { Registry } from "../lib/api/types/registries";

  let registries = $state<Registry[]>([]);
  let loading = $state(true);
  let loaded = $state(false);
  let error = $state<string | null>(null);
  let showCreateModal = $state(false);
  let editingRegistry = $state<Registry | null>(null);
  let deletingRegistry = $state<Registry | null>(null);
  let deleteError = $state<string | null>(null);
  let deleting = $state(false);

  async function load() {
    loading = true;
    error = null;

    try {
      registries = await listRegistries();
    } catch (err) {
      error =
        err instanceof ApiError ? err.message : "Failed to load registries";
    } finally {
      loading = false;
      loaded = true;
    }
  }

  onMount(load);

  async function handleCreated(registry: Registry) {
    await load();
    toast.success(`Registry "${registry.registry}" created.`);
  }

  async function handleUpdated(registry: Registry) {
    await load();
    toast.success(`Registry "${registry.registry}" updated.`);
  }

  function cancelDelete() {
    deletingRegistry = null;
    deleteError = null;
  }

  async function handleDelete() {
    if (!deletingRegistry) return;

    deleteError = null;
    deleting = true;

    try {
      const deletedName = deletingRegistry.registry;
      await deleteRegistry(deletingRegistry.id);
      deletingRegistry = null;
      await load();
      toast.success(`Registry "${deletedName}" deleted.`);
    } catch (err) {
      deleteError =
        err instanceof ApiError ? err.message : "Failed to delete registry";
    } finally {
      deleting = false;
    }
  }
</script>

<PageTitle title="Registries" />

<AppShell>
  <div class="list-header">
    <div class="title-row">
      <Server size={20} strokeWidth={1.75} />
      <h1>Registries</h1>
    </div>
    <Button onclick={() => (showCreateModal = true)}>
      <Plus size={16} strokeWidth={2} />
      Add registry
    </Button>
  </div>

  <AsyncState
    loading={loading && !loaded}
    busy={loading && loaded}
    {error}
    empty={registries.length === 0}
    emptyMessage="No registries yet. Add one to start tracking virtual images."
    columns={5}
  >
    {#snippet emptyAction()}
      <Button onclick={() => (showCreateModal = true)}>
        <Plus size={16} strokeWidth={2} />
        Add registry
      </Button>
    {/snippet}

    <div class="card table-wrap">
      <table class="table">
        <thead>
          <tr>
            <th>Registry</th>
            <th>Username</th>
            <th>Authentication</th>
            <th>Options</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each registries as registry (registry.id)}
            <tr>
              <td>{registry.registry}</td>
              <td>{registry.username ?? "-"}</td>
              <td>
                {#if registry.authenticationConfigured}
                  <span class="badge badge-accent">Configured</span>
                {:else}
                  <span class="badge">None</span>
                {/if}
              </td>
              <td>
                {#if registry.options.insecure}
                  <span class="badge badge-warning">Insecure</span>
                {/if}
                {#if registry.options.allow_self_signed_certs}
                  <span class="badge badge-warning">Self-signed</span>
                {/if}
                {#if !registry.options.insecure && !registry.options.allow_self_signed_certs}
                  <span class="muted">-</span>
                {/if}
              </td>
              <td class="actions">
                <button
                  type="button"
                  class="icon-button"
                  onclick={() => (editingRegistry = registry)}
                  aria-label="Edit {registry.registry}"
                >
                  <Pencil size={16} strokeWidth={1.75} />
                </button>
                <button
                  type="button"
                  class="icon-button danger"
                  onclick={() => (deletingRegistry = registry)}
                  aria-label="Delete {registry.registry}"
                >
                  <Trash2 size={16} strokeWidth={1.75} />
                </button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </AsyncState>
</AppShell>

<CreateRegistryModal
  open={showCreateModal}
  onClose={() => (showCreateModal = false)}
  onCreated={handleCreated}
/>

<EditRegistryModal
  open={editingRegistry !== null}
  registry={editingRegistry}
  onClose={() => (editingRegistry = null)}
  onUpdated={handleUpdated}
/>

<ConfirmDialog
  open={deletingRegistry !== null}
  title="Delete registry"
  message={`Delete "${deletingRegistry?.registry ?? ""}"? This cannot be undone.`}
  confirmLabel="Delete"
  danger
  error={deleteError}
  submitting={deleting}
  onConfirm={handleDelete}
  onCancel={cancelDelete}
/>

<style>
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-1);
  }
</style>
