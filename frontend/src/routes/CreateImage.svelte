<script lang="ts">
  import { onMount } from "svelte";
  import { push } from "svelte-spa-router";
  import PackagePlus from "@lucide/svelte/icons/package-plus";
  import Check from "@lucide/svelte/icons/check";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import AppShell from "../lib/components/AppShell.svelte";
  import AsyncState from "../lib/components/AsyncState.svelte";
  import Field from "../lib/components/Field.svelte";
  import Select from "../lib/components/Select.svelte";
  import Button from "../lib/components/Button.svelte";
  import TagFamilyPicker from "../lib/components/TagFamilyPicker.svelte";
  import { listRegistries } from "../lib/api/registries";
  import { inspectRepository, createImage } from "../lib/api/images";
  import { ApiError } from "../lib/api/client";
  import { toast } from "../lib/services/toast";
  import type { Registry } from "../lib/api/types/registries";
  import type { TagGroup } from "../lib/api/types/images";

  const STEP_LABELS: Record<1 | 2, string> = {
    1: "Source",
    2: "Choose a starting tag",
  };

  let registries = $state<Registry[]>([]);
  let loadingRegistries = $state(true);
  let registriesError = $state<string | null>(null);

  let step = $state<1 | 2>(1);

  let name = $state("");
  let registryId = $state("");
  let repository = $state("");
  let inspecting = $state(false);
  let inspectError = $state<string | null>(null);

  let tagGroups = $state<TagGroup[]>([]);
  let selectedTag = $state<string | null>(null);
  let creating = $state(false);
  let createError = $state<string | null>(null);

  async function loadRegistries() {
    loadingRegistries = true;
    registriesError = null;

    try {
      registries = await listRegistries();
    } catch (err) {
      registriesError =
        err instanceof ApiError ? err.message : "Failed to load registries";
    } finally {
      loadingRegistries = false;
    }
  }

  onMount(loadRegistries);

  const registryOptions = $derived(
    registries.map((registry) => ({
      value: String(registry.id),
      label: registry.registry,
    })),
  );
  const selectedRegistry = $derived(
    registries.find((r) => r.id === Number(registryId)),
  );

  async function handleInspect(event: SubmitEvent) {
    event.preventDefault();

    if (!selectedRegistry) return;

    inspectError = null;
    inspecting = true;

    try {
      const result = await inspectRepository({
        registry: selectedRegistry.registry,
        repository: repository.trim(),
      });
      tagGroups = result.tagGroups;
      selectedTag = null;
      step = 2;
    } catch (err) {
      inspectError =
        err instanceof ApiError ? err.message : "Failed to inspect repository";
    } finally {
      inspecting = false;
    }
  }

  function backToSource() {
    step = 1;
    createError = null;
  }

  async function handleCreate() {
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
      push("/images");
    } catch (err) {
      createError =
        err instanceof ApiError
          ? err.message
          : "Failed to create virtual image";
    } finally {
      creating = false;
    }
  }
</script>

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
          <form onsubmit={handleInspect}>
            <Field
              label="Virtual image name"
              bind:value={name}
              placeholder="myimage"
              required
            />
            <Select
              label="Registry"
              bind:value={registryId}
              options={registryOptions}
              placeholder="Select a registry…"
              required
            />
            <Field
              label="Repository"
              bind:value={repository}
              placeholder="library/nginx"
              required
            />

            {#if inspectError}
              <p class="error">
                <TriangleAlert size={16} strokeWidth={2} />
                {inspectError}
              </p>
            {/if}

            <Button type="submit" disabled={inspecting}>
              {inspecting ? "Checking…" : "Check repository"}
            </Button>
          </form>
        {:else}
          <p class="recap">
            <span class="recap-text">
              <strong>{selectedRegistry?.registry}</strong> / {repository}
            </span>
            <button type="button" class="link" onclick={backToSource}
              >Edit</button
            >
          </p>

          <TagFamilyPicker
            {tagGroups}
            bind:selectedTag
            emptyMessage="No tags were found for this repository."
          />

          {#if createError}
            <p class="error tag-error">
              <TriangleAlert size={16} strokeWidth={2} />
              {createError}
            </p>
          {/if}

          <div class="create-bar">
            <span class="selection">
              {#if selectedTag}
                Selected tag: <span class="tag-value">{selectedTag}</span>
              {:else}
                <span class="muted">Pick a tag above to continue</span>
              {/if}
            </span>
            <Button disabled={!selectedTag || creating} onclick={handleCreate}>
              {creating ? "Creating…" : "Create virtual image"}
            </Button>
          </div>
        {/if}
      </div>
    </AsyncState>
  </div>
</AppShell>

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
    white-space: nowrap;
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

  .recap {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    margin: 0 0 var(--space-5);
    padding-bottom: var(--space-4);
    border-bottom: 1px solid var(--color-border);
    font-size: 0.875rem;
  }

  .recap-text {
    font-family: monospace;
    color: var(--color-text-muted);
  }

  .link {
    flex-shrink: 0;
  }

  .create-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    margin-top: var(--space-5);
  }

  .tag-error {
    margin-top: var(--space-4);
  }

  .selection {
    font-size: 0.875rem;
  }
</style>
