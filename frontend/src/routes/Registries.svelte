<script lang="ts">
  import { onMount } from "svelte";
  import Plus from "@lucide/svelte/icons/plus";
  import Server from "@lucide/svelte/icons/server";
  import AppShell from "../lib/components/AppShell.svelte";
  import PageTitle from "../lib/components/PageTitle.svelte";
  import AsyncState from "../lib/components/AsyncState.svelte";
  import Button from "../lib/components/Button.svelte";
  import RegistryModal from "../lib/components/RegistryModal.svelte";
  import { listRegistries } from "../lib/api/registries";
  import { errorMessage } from "../lib/api/client";
  import { toast } from "../lib/services/toast";
  import type { Registry } from "../lib/api/types/registries";

  let registries = $state<Registry[]>([]);
  let loading = $state(true);
  let loaded = $state(false);
  let error = $state<string | null>(null);
  let showCreateModal = $state(false);
  let editingRegistry = $state<Registry | null>(null);
  let loadToken = 0;

  async function load() {
    const requestId = ++loadToken;
    loading = true;
    error = null;

    try {
      const result = await listRegistries();
      if (requestId !== loadToken) return;
      registries = result;
    } catch (err) {
      if (requestId !== loadToken) return;
      error = errorMessage(err, "Failed to load registries");
    } finally {
      if (requestId === loadToken) {
        loading = false;
        loaded = true;
      }
    }
  }

  onMount(load);

  async function handleSaved(registry: Registry, action: "created" | "updated") {
    await load();
    toast.success(`Registry "${registry.registry}" ${action}.`);
  }

  function handleRowClick(event: MouseEvent, registry: Registry) {
    if ((event.target as HTMLElement).closest("a, button")) return;
    editingRegistry = registry;
  }

  async function handleDeleted(registry: Registry) {
    editingRegistry = null;
    await load();
    toast.success(`Registry "${registry.registry}" deleted.`);
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
    columns={4}
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
            <th class="phone-hidden">Username</th>
            <th>Authentication</th>
            <th class="phone-hidden">Options</th>
          </tr>
        </thead>
        <tbody>
          {#each registries as registry (registry.id)}
            <tr class="clickable" onclick={(event) => handleRowClick(event, registry)}>
              <td class="host-cell">
                <button
                  type="button"
                  class="registry-name"
                  onclick={() => (editingRegistry = registry)}>{registry.registry}</button
                >
              </td>
              <td class="phone-hidden">{registry.username ?? "-"}</td>
              <td>
                {#if registry.authenticationConfigured}
                  <span class="badge badge-accent">Configured</span>
                {:else}
                  <span class="badge">None</span>
                {/if}
              </td>
              <td class="phone-hidden">
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
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </AsyncState>
</AppShell>

<RegistryModal
  mode="create"
  open={showCreateModal}
  onClose={() => (showCreateModal = false)}
  onSaved={(registry) => handleSaved(registry, "created")}
/>

<RegistryModal
  mode="edit"
  open={editingRegistry !== null}
  registry={editingRegistry}
  onClose={() => (editingRegistry = null)}
  onSaved={(registry) => handleSaved(registry, "updated")}
  onDeleted={handleDeleted}
/>

<style>
  .host-cell {
    overflow-wrap: anywhere;
  }

  .registry-name {
    padding: 0;
    overflow-wrap: anywhere;
    border: none;
    background: none;
    color: inherit;
    font: inherit;
    font-weight: 500;
    text-align: left;
    cursor: pointer;
  }
</style>
