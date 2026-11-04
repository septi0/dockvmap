<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { push } from "svelte-spa-router";
  import PackagePlus from "@lucide/svelte/icons/package-plus";
  import Check from "@lucide/svelte/icons/check";
  import Plus from "@lucide/svelte/icons/plus";
  import AppShell from "../lib/components/AppShell.svelte";
  import PageTitle from "../lib/components/PageTitle.svelte";
  import AsyncState from "../lib/components/AsyncState.svelte";
  import Button from "../lib/components/Button.svelte";
  import SourceStep from "../lib/components/createImage/SourceStep.svelte";
  import TagSelectionStep from "../lib/components/createImage/TagSelectionStep.svelte";
  import RegistryModal from "../lib/components/RegistryModal.svelte";
  import { listInternalRepositories, listRegistries } from "../lib/api/registries";
  import { INTERNAL_REGISTRY_LABEL } from "../lib/utils/registries";
  import { createImage } from "../lib/api/images";
  import { errorMessage } from "../lib/api/client";
  import { toast } from "../lib/services/toast";
  import { createTagDiscovery } from "../lib/services/tagDiscovery";
  import type { Registry } from "../lib/api/types/registries";
  import type { TagGroup, DiscoveryResult } from "../lib/api/types/images";

  const STEP_LABELS: Record<1 | 2, string> = {
    1: "Source",
    2: "Choose a starting tag",
  };

  const discovery = createTagDiscovery();

  let registries = $state<Registry[]>([]);
  let loadingRegistries = $state(true);
  let registriesError = $state<string | null>(null);
  let showRegistryModal = $state(false);

  let step = $state<1 | 2>(1);

  let name = $state("");
  let registryId = $state("");
  let repository = $state("");
  let lastAutoName = $state("");

  let tagGroups = $state<TagGroup[]>([]);
  let tagCount = $state(0);
  let ignoredCount = $state(0);
  let selectedTag = $state<string | null>(null);
  let creating = $state(false);
  let createError = $state<string | null>(null);
  let addingAnother = $state(false);
  let internalRepositories = $state<string[]>([]);

  async function loadRegistries() {
    loadingRegistries = true;
    registriesError = null;

    try {
      registries = await listRegistries();

      if (registries.some((registry) => registry.internal)) {
        listInternalRepositories()
          .then((names) => (internalRepositories = names))
          .catch(() => {});
      }
    } catch (err) {
      registriesError = errorMessage(err, "Failed to load registries");
    } finally {
      loadingRegistries = false;
    }
  }

  async function handleRegistryCreated(registry: Registry) {
    showRegistryModal = false;
    toast.success(`Registry "${registry.registry}" created.`);
    await loadRegistries();
    registryId = String(registry.id);
  }

  onMount(loadRegistries);
  onMount(() => discovery.onResolved(applyDiscovery));
  onDestroy(discovery.destroy);

  const registryOptions = $derived(
    [...registries]
      .sort((a, b) => Number(b.internal) - Number(a.internal))
      .map((registry) => ({
        value: String(registry.id),
        label: registry.internal ? `${INTERNAL_REGISTRY_LABEL} (${registry.registry})` : registry.registry,
      })),
  );
  const selectedRegistry = $derived(
    registries.find((r) => r.id === Number(registryId)),
  );

  function deriveNameFromRepository(value: string): string {
    return value.trim().split("/").pop() ?? "";
  }

  $effect(() => {
    const derived = deriveNameFromRepository(repository);

    if (name === lastAutoName) {
      name = derived;
      lastAutoName = derived;
    }
  });

  function applyDiscovery(result: DiscoveryResult) {
    tagGroups = result.tagGroups ?? [];
    tagCount = result.tagCount ?? 0;
    ignoredCount = result.ignoredCount ?? 0;
    selectedTag = null;
    step = 2;
  }

  function handleInspect(event: SubmitEvent) {
    event.preventDefault();

    if (!selectedRegistry) return;

    void discovery.start({
      registry: selectedRegistry.registry,
      repository: repository.trim(),
    });
  }

  function backToSource() {
    discovery.cancel();
    step = 1;
    createError = null;
  }

  function startAnother() {
    const slash = repository.lastIndexOf("/");

    discovery.cancel();
    repository = repository.slice(0, slash + 1);
    name = "";
    lastAutoName = "";
    selectedTag = null;
    tagGroups = [];
    addingAnother = true;
    step = 1;
  }

  async function handleCreate(addAnother = false) {
    if (!selectedTag) return;

    createError = null;
    creating = true;

    try {
      await createImage({
        name: name.trim(),
        registryId: Number(registryId),
        repository: repository.trim(),
        tag: selectedTag,
      });
      toast.success(`Virtual image "${name.trim()}" created.`);

      if (addAnother) startAnother();
      else push("/images");
    } catch (err) {
      createError = errorMessage(err, "Failed to create virtual image");
    } finally {
      creating = false;
    }
  }
</script>

<PageTitle title="Add Virtual Image" />

<AppShell>
  <div class="page">
    <div class="page-header">
      <div class="title-row">
        <PackagePlus size={20} strokeWidth={1.75} />
        <h1>Add virtual image</h1>
      </div>
      <p class="subtitle">
        Track a repository through a stable tag that always resolves to the
        version you choose.
      </p>
    </div>

    <AsyncState
      loading={loadingRegistries}
      error={registriesError}
      empty={registries.length === 0}
      emptyMessage="No registries yet. Add one before creating a virtual image."
    >
      {#snippet emptyAction()}
        <Button onclick={() => (showRegistryModal = true)}>
          <Plus size={16} strokeWidth={2} />
          Add registry
        </Button>
      {/snippet}

      <div class="stepper">
        <div class="stepper-step">
          <span
            class="stepper-circle"
            class:active={step === 1}
            class:done={step === 2}
          >
            {#if step === 2}
              <Check size={14} strokeWidth={3} />
            {:else}
              1
            {/if}
          </span>
          <span class="stepper-label" class:active={step === 1}
            >{STEP_LABELS[1]}</span
          >
        </div>
        <span class="stepper-connector" class:done={step === 2}></span>
        <div class="stepper-step">
          <span class="stepper-circle" class:active={step === 2}>2</span>
          <span class="stepper-label" class:active={step === 2}
            >{STEP_LABELS[2]}</span
          >
        </div>
      </div>

      <div class="card wizard-card">
        {#if step === 1}
          <SourceStep
            bind:name
            bind:registryId
            bind:repository
            {registryOptions}
            registryLabel={selectedRegistry?.registry}
            repositorySuggestions={selectedRegistry?.internal ? internalRepositories : []}
            focusRepository={addingAnother}
            discovery={$discovery}
            onSubmit={handleInspect}
            onCancelDiscovery={discovery.cancel}
          />
        {:else}
          <TagSelectionStep
            registryLabel={selectedRegistry?.registry}
            {repository}
            {tagGroups}
            {tagCount}
            {ignoredCount}
            bind:selectedTag
            {creating}
            error={createError}
            onBack={backToSource}
            onCreate={() => handleCreate()}
            onCreateAnother={() => handleCreate(true)}
          />
        {/if}
      </div>
    </AsyncState>
  </div>
</AppShell>

<RegistryModal
  mode="create"
  open={showRegistryModal}
  onClose={() => (showRegistryModal = false)}
  onSaved={handleRegistryCreated}
/>

<style>
  .page {
    max-width: 640px;
    margin: 0 auto;
  }

  .stepper {
    display: flex;
    align-items: center;
    margin-bottom: var(--space-6);
  }

  .stepper-step {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-shrink: 0;
  }

  .stepper-circle {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border-radius: var(--radius-full);
    border: 1px solid var(--color-border-strong);
    background: var(--color-surface);
    color: var(--color-text-faint);
    font-size: 0.8125rem;
    font-weight: 600;
    flex-shrink: 0;
  }

  .stepper-circle.active {
    border-color: var(--color-accent);
    background: var(--color-accent);
    color: var(--color-accent-contrast);
  }

  .stepper-circle.done {
    border-color: var(--color-accent);
    background: var(--color-accent-muted-bg);
    color: var(--color-accent);
  }

  .stepper-label {
    font-size: 0.875rem;
    color: var(--color-text-faint);
  }

  .stepper-label.active {
    color: var(--color-text);
    font-weight: 600;
  }

  .stepper-connector {
    flex: 1;
    height: 1px;
    margin: 0 var(--space-3);
    background: var(--color-border);
  }

  .stepper-connector.done {
    background: var(--color-accent);
  }

  .wizard-card {
    padding: var(--space-6);
  }
</style>
