<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import FolderSearch from "@lucide/svelte/icons/folder-search";
  import Plus from "@lucide/svelte/icons/plus";
  import Server from "@lucide/svelte/icons/server";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import AppShell from "../lib/components/AppShell.svelte";
  import PageTitle from "../lib/components/PageTitle.svelte";
  import AsyncState from "../lib/components/AsyncState.svelte";
  import Button from "../lib/components/Button.svelte";
  import RegistryModal from "../lib/components/RegistryModal.svelte";
  import { listBrowsableRegistries, listRegistries } from "../lib/api/registries";
  import { errorMessage } from "../lib/api/client";
  import { toast } from "../lib/services/toast";
  import type { Registry } from "../lib/api/types/registries";

  let registries = $state<Registry[]>([]);
  let loading = $state(true);
  let loaded = $state(false);
  let error = $state<string | null>(null);
  let showCreateModal = $state(false);
  let editingRegistry = $state<Registry | null>(null);
  let browsable = $state<Set<number>>(new Set());
  let loadToken = 0;

  async function load() {
    const requestId = ++loadToken;
    loading = true;
    error = null;

    listBrowsableRegistries()
      .then((result) => {
        if (requestId === loadToken) browsable = new Set(result.registryIds);
      })
      .catch(() => {});

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

  function imagesHref(registry: Registry) {
    return `/images?registry=${registry.id}`;
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
            <th class="phone-hidden">Virtual images</th>
            <th>Authentication</th>
            <th class="phone-hidden">Options</th>
            <th class="browse-cell"></th>
          </tr>
        </thead>
        <tbody>
          {#each registries as registry (registry.id)}
            <tr class="clickable" onclick={(event) => handleRowClick(event, registry)}>
              <td class="host-cell">
                <span class="name-cell">
                  <button
                    type="button"
                    class="row-name registry-name"
                    onclick={() => (editingRegistry = registry)}>{registry.registry}</button
                  >
                  {#if registry.internal}<span class="badge badge-accent">Internal</span>{/if}
                </span>
                {#if registry.internal}
                  <p class="muted registry-note">Set by <code class="inline-code">internal_registry</code> in config</p>
                {/if}
                {#if registry.failedChecks > 0}
                  <p class="warning-text registry-note">
                    <TriangleAlert size={14} strokeWidth={2} />
                    <a class="row-link accent" href={`${imagesHref(registry)}&status=failedCheck`} use:link
                      >{registry.failedChecks} failing {registry.failedChecks === 1 ? "check" : "checks"}</a
                    >
                  </p>
                {/if}
                {#if registry.imageCount > 0}
                  <p class="phone-only registry-note">
                    <a class="row-link accent" href={imagesHref(registry)} use:link
                      >{registry.imageCount} virtual {registry.imageCount === 1 ? "image" : "images"}</a
                    >
                  </p>
                {/if}
              </td>
              <td class="phone-hidden">
                {#if registry.imageCount > 0}
                  <a
                    class="row-link accent"
                    href={imagesHref(registry)}
                    title="Show the virtual images from this registry"
                    use:link>{registry.imageCount}</a
                  >
                {:else}
                  <span class="muted">0</span>
                {/if}
              </td>
              <td class="auth-cell">
                {#if registry.authenticationConfigured}
                  {registry.username}
                {:else}
                  <span class="muted">None</span>
                {/if}
              </td>
              <td class="phone-hidden">
                {#if registry.internal && registry.options.insecure}
                  <span class="muted">Stack network</span>
                {:else if registry.options.insecure}
                  <span class="badge badge-warning">Insecure</span>
                {/if}
                {#if registry.options.allow_self_signed_certs}
                  <span class="badge badge-warning">Self-signed</span>
                {/if}
                {#if !registry.options.insecure && !registry.options.allow_self_signed_certs}
                  <span class="muted">-</span>
                {/if}
              </td>
              <td class="browse-cell">
                <div class="table-actions">
                  {#if browsable.has(registry.id)}
                    <a
                      class="icon-button"
                      href={`/registries/${registry.id}/browse`}
                      title="Browse"
                      aria-label={`Browse ${registry.registry}`}
                      use:link
                    >
                      <FolderSearch size={16} strokeWidth={1.75} />
                    </a>
                  {/if}
                </div>
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

  .auth-cell {
    overflow-wrap: anywhere;
  }

  .browse-cell {
    width: 48px;
  }

  .registry-note {
    margin: 2px 0 0;
    font-size: 0.8125rem;
  }

  .registry-name {
    overflow-wrap: anywhere;
  }
</style>
