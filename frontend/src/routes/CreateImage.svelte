<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { push } from "svelte-spa-router";
  import PackagePlus from "@lucide/svelte/icons/package-plus";
  import Check from "@lucide/svelte/icons/check";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import LoaderCircle from "@lucide/svelte/icons/loader-circle";
  import AppShell from "../lib/components/AppShell.svelte";
  import AsyncState from "../lib/components/AsyncState.svelte";
  import Field from "../lib/components/Field.svelte";
  import Select from "../lib/components/Select.svelte";
  import Button from "../lib/components/Button.svelte";
  import TagFamilyPicker from "../lib/components/TagFamilyPicker.svelte";
  import { listRegistries } from "../lib/api/registries";
  import { inspectRepository, getDiscovery, createImage } from "../lib/api/images";
  import { ApiError } from "../lib/api/client";
  import { toast } from "../lib/services/toast";
  import type { Registry } from "../lib/api/types/registries";
  import type { TagGroup, DiscoveryResult } from "../lib/api/types/images";

  const DISCOVERY_POLL_INTERVAL_MS = 1000;
  const MAX_CONSECUTIVE_POLL_ERRORS = 3;

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
  let discovering = $state(false);
  let inspectError = $state<string | null>(null);

  let pollToken = 0;
  let pollTimer: ReturnType<typeof setTimeout> | null = null;

  let elapsedSeconds = $state(0);
  let elapsedTimer: ReturnType<typeof setInterval> | null = null;
  let tagsSeenSoFar = $state(0);

  let tagGroups = $state<TagGroup[]>([]);
  let tagCount = $state(0);
  let ignoredCount = $state(0);
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

  const formattedElapsed = $derived(
    `${Math.floor(elapsedSeconds / 60)}:${String(elapsedSeconds % 60).padStart(2, "0")}`,
  );

  const discoverySubtitle = $derived(
    tagsSeenSoFar > 0
      ? `${tagsSeenSoFar.toLocaleString()} tags discovered so far…`
      : `Fetching tags from ${selectedRegistry?.registry}/${repository}…`,
  );

  const showLeavePageHint = $derived(elapsedSeconds >= 15);

  function startElapsedTimer() {
    stopElapsedTimer();
    elapsedSeconds = 0;
    tagsSeenSoFar = 0;
    elapsedTimer = setInterval(() => {
      elapsedSeconds++;
    }, 1000);
  }

  function stopElapsedTimer() {
    if (elapsedTimer) {
      clearInterval(elapsedTimer);
      elapsedTimer = null;
    }
  }

  function stopPolling() {
    pollToken++;

    if (pollTimer) {
      clearTimeout(pollTimer);
      pollTimer = null;
    }
  }

  function applyDiscovery(result: DiscoveryResult) {
    discovering = false;
    stopElapsedTimer();

    if (result.status === "failed") {
      inspectError = result.error || "Tag discovery failed";
      return;
    }

    tagGroups = result.tagGroups ?? [];
    tagCount = result.tagCount ?? 0;
    ignoredCount = result.ignoredCount ?? 0;
    selectedTag = null;
    step = 2;
  }

  function pollDiscovery(id: number) {
    const token = ++pollToken;
    let consecutiveErrors = 0;

    const tick = async () => {
      if (token !== pollToken) return;

      try {
        const result = await getDiscovery(id);
        if (token !== pollToken) return;

        if (result.status === "running") {
          consecutiveErrors = 0;
          tagsSeenSoFar = result.tagsSeen ?? tagsSeenSoFar;
          pollTimer = setTimeout(tick, DISCOVERY_POLL_INTERVAL_MS);
          return;
        }

        applyDiscovery(result);
      } catch (err) {
        if (token !== pollToken) return;

        consecutiveErrors++;

        if (consecutiveErrors < MAX_CONSECUTIVE_POLL_ERRORS) {
          pollTimer = setTimeout(tick, DISCOVERY_POLL_INTERVAL_MS);
          return;
        }

        discovering = false;
        stopElapsedTimer();
        inspectError =
          err instanceof ApiError ? err.message : "Failed to check discovery status";
      }
    };

    pollTimer = setTimeout(tick, DISCOVERY_POLL_INTERVAL_MS);
  }

  async function handleInspect(event: SubmitEvent) {
    event.preventDefault();

    if (!selectedRegistry) return;

    stopPolling();
    inspectError = null;
    inspecting = true;
    discovering = false;

    try {
      const result = await inspectRepository({
        registry: selectedRegistry.registry,
        repository: repository.trim(),
      });

      if (result.status === "running") {
        discovering = true;
        startElapsedTimer();
        tagsSeenSoFar = result.tagsSeen ?? 0;
        pollDiscovery(result.id);
      } else {
        applyDiscovery(result);
      }
    } catch (err) {
      inspectError =
        err instanceof ApiError ? err.message : "Failed to inspect repository";
    } finally {
      inspecting = false;
    }
  }

  function cancelDiscovery() {
    stopPolling();
    stopElapsedTimer();
    discovering = false;
  }

  onDestroy(() => {
    stopPolling();
    stopElapsedTimer();
  });

  function backToSource() {
    stopPolling();
    stopElapsedTimer();
    discovering = false;
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
              disabled={inspecting || discovering}
            />
            <Select
              label="Registry"
              bind:value={registryId}
              options={registryOptions}
              placeholder="Select a registry…"
              required
              disabled={inspecting || discovering}
            />
            <Field
              label="Repository"
              bind:value={repository}
              placeholder="library/nginx"
              required
              disabled={inspecting || discovering}
            />

            {#if discovering}
              <div class="discovery-panel" role="status">
                <span class="discovery-icon">
                  <LoaderCircle size={18} strokeWidth={2.5} />
                </span>
                <div class="discovery-body">
                  <p class="discovery-title">
                    Discovering tags…
                    <span class="discovery-elapsed">{formattedElapsed}</span>
                  </p>
                  <p class="discovery-subtitle">{discoverySubtitle}</p>
                  {#if showLeavePageHint}
                    <p class="discovery-hint">
                      It's safe to leave this page: discovery keeps running in
                      the background.
                    </p>
                  {/if}
                </div>
                <button
                  type="button"
                  class="link discovery-cancel"
                  onclick={cancelDiscovery}
                >
                  Cancel
                </button>
              </div>
            {/if}

            {#if inspectError}
              <p class="error">
                <TriangleAlert size={16} strokeWidth={2} />
                {inspectError}
              </p>
            {/if}

            {#if !discovering}
              <Button type="submit" disabled={inspecting}>
                {inspecting ? "Checking…" : "Check repository"}
              </Button>
            {/if}
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

          <p class="discovery-summary">
            {tagCount}
            {tagCount === 1 ? "tag" : "tags"} found
            {#if ignoredCount > 0}
              &middot; {ignoredCount} filtered out
            {/if}
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

  .discovery-panel {
    display: flex;
    align-items: flex-start;
    gap: var(--space-3);
    margin: var(--space-4) 0 0;
    padding: var(--space-4);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    background: var(--color-accent-muted-bg);
  }

  .discovery-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border-radius: var(--radius-full);
    background: var(--color-surface);
    color: var(--color-accent);
    flex-shrink: 0;
    animation: spin 0.8s linear infinite;
  }

  .discovery-body {
    flex: 1;
    min-width: 0;
  }

  .discovery-title {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin: 0;
    font-size: 0.875rem;
    font-weight: 600;
    color: var(--color-text);
  }

  .discovery-elapsed {
    font-family: monospace;
    font-size: 0.75rem;
    font-weight: 500;
    color: var(--color-text-muted);
    background: var(--color-surface);
    padding: 1px var(--space-2);
    border-radius: var(--radius-full);
    border: 1px solid var(--color-border);
  }

  .discovery-subtitle {
    margin: var(--space-1) 0 0;
    font-size: 0.8125rem;
    color: var(--color-text-muted);
    line-height: 1.4;
  }

  .discovery-hint {
    margin: var(--space-2) 0 0;
    font-size: 0.75rem;
    color: var(--color-text-faint);
    line-height: 1.4;
  }

  .discovery-cancel {
    flex-shrink: 0;
    align-self: center;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
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

  .discovery-summary {
    margin: 0 0 var(--space-4);
    font-size: 0.8125rem;
    color: var(--color-text-faint);
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
