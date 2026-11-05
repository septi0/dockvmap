<script lang="ts">
  import { link } from "svelte-spa-router";
  import ArrowLeft from "@lucide/svelte/icons/arrow-left";
  import FolderSearch from "@lucide/svelte/icons/folder-search";
  import AppShell from "../lib/components/AppShell.svelte";
  import PageTitle from "../lib/components/PageTitle.svelte";
  import AsyncState from "../lib/components/AsyncState.svelte";
  import FilterBar from "../lib/components/FilterBar.svelte";
  import SearchInput from "../lib/components/SearchInput.svelte";
  import RepositoryTagsModal from "../lib/components/RepositoryTagsModal.svelte";
  import { listRegistries, listRegistryRepositories } from "../lib/api/registries";
  import { errorMessage } from "../lib/api/client";
  import type { Registry, RegistryRepositories, RegistryRepository } from "../lib/api/types/registries";

  let { params }: { params: { id: string } } = $props();
  const registryId = $derived(Number(params.id));

  let registry = $state<Registry | null>(null);
  let result = $state<RegistryRepositories | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let search = $state("");
  let withoutImages = $state(false);
  let openRepository = $state<RegistryRepository | null>(null);

  let repositories = $derived.by(() => {
    const term = search.trim().toLowerCase();

    return (result?.repositories ?? []).filter(
      (repository) =>
        (!term || repository.name.toLowerCase().includes(term)) &&
        (!withoutImages || repository.images.length === 0),
    );
  });

  let filtersActive = $derived(!!search || withoutImages);

  async function load(id: number) {
    loading = true;
    error = null;

    listRegistries()
      .then((registries) => (registry = registries.find((r) => r.id === id) ?? null))
      .catch(() => {});

    try {
      result = await listRegistryRepositories(id);
    } catch (err) {
      error = errorMessage(err, "Failed to list repositories");
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void load(registryId);
  });

  function clearFilters() {
    search = "";
    withoutImages = false;
  }

  function handleRowClick(event: MouseEvent, repository: RegistryRepository) {
    if ((event.target as HTMLElement).closest("a, button")) return;
    openRepository = repository;
  }
</script>

{#snippet virtualImages(repository: RegistryRepository)}
  {#if repository.images.length === 0}
    <span class="muted">-</span>
  {:else}
    {#each repository.images as image, index (image.id)}
      {#if index > 0},
      {/if}<a class="row-link accent" href={`/images/${image.id}`} use:link>{image.name}</a>
    {/each}
  {/if}
{/snippet}

<PageTitle title={registry ? `Browse ${registry.registry}` : "Browse registry"} />

<AppShell>
  <a class="back-link" href="/registries" use:link>
    <ArrowLeft size={14} strokeWidth={2} />
    Registries
  </a>

  <div class="page-header">
    <div class="title-row">
      <FolderSearch size={20} strokeWidth={1.75} />
      <h1>{registry?.registry ?? "Browse registry"}</h1>
      {#if registry?.internal}<span class="badge badge-accent">Internal</span>{/if}
    </div>
  </div>

  <AsyncState loading={loading && !result} {error} columns={2}>
    {#if result}
      <div class="card section-card">
        <h2 class="heading-with-count">
          Repositories <span class="muted heading-count">{result.repositories.length}</span>
        </h2>

        {#if result.repositories.length === 0}
          <p class="muted">No repositories yet.</p>
        {:else}
          {#if result.more}
            <p class="muted more-note">
              Showing the first {result.repositories.length} repositories by name; search covers only these.
            </p>
          {/if}

          <FilterBar active={filtersActive} onClear={clearFilters}>
            <SearchInput
              bind:value={search}
              placeholder="Search repositories…"
              onSearch={() => {}}
              delay={0}
            />
            <div class="filter-field">
              <span class="filter-label">Show</span>
              <select
                class="input filter-control"
                class:is-active={withoutImages}
                bind:value={withoutImages}
              >
                <option value={false}>All repositories</option>
                <option value={true}>No virtual image</option>
              </select>
            </div>
          </FilterBar>

          {#if repositories.length === 0}
            <p class="muted">No repositories match these filters.</p>
          {:else}
            <div class="table-wrap">
              <table class="table">
                <thead>
                  <tr>
                    <th>Repository</th>
                    <th class="phone-hidden">Virtual images</th>
                  </tr>
                </thead>
                <tbody>
                  {#each repositories as repository (repository.name)}
                    <tr class="clickable" onclick={(event) => handleRowClick(event, repository)}>
                      <td>
                        <button
                          type="button"
                          class="row-name"
                          onclick={() => (openRepository = repository)}>{repository.name}</button
                        >
                        {#if repository.images.length > 0}
                          <div class="phone-only muted images-under">
                            {@render virtualImages(repository)}
                          </div>
                        {/if}
                      </td>
                      <td class="phone-hidden">{@render virtualImages(repository)}</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
        {/if}
      </div>
    {/if}
  </AsyncState>
</AppShell>

{#if registry}
  <RepositoryTagsModal
    {registry}
    repository={openRepository}
    onClose={() => (openRepository = null)}
  />
{/if}

<style>
  .images-under {
    margin-top: var(--space-1);
    font-size: 0.8125rem;
  }

  .more-note {
    margin: 0 0 var(--space-3);
    font-size: 0.8125rem;
  }
</style>
