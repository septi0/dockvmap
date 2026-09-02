<script lang="ts">
  import LoaderCircle from "@lucide/svelte/icons/loader-circle";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Field from "../Field.svelte";
  import Select from "../Select.svelte";
  import Button from "../Button.svelte";
  import type { TagDiscoveryState } from "../../services/tagDiscovery";

  let {
    name = $bindable(),
    registryId = $bindable(),
    repository = $bindable(),
    registryOptions,
    registryLabel,
    discovery,
    onSubmit,
    onCancelDiscovery,
  }: {
    name: string;
    registryId: string;
    repository: string;
    registryOptions: { value: string; label: string }[];
    registryLabel: string | undefined;
    discovery: TagDiscoveryState;
    onSubmit: (event: SubmitEvent) => void;
    onCancelDiscovery: () => void;
  } = $props();

  let inspecting = $derived(discovery.phase === "inspecting");
  let discovering = $derived(discovery.phase === "discovering");
  let locked = $derived(inspecting || discovering);

  let formattedElapsed = $derived(
    `${Math.floor(discovery.elapsedSeconds / 60)}:${String(
      discovery.elapsedSeconds % 60,
    ).padStart(2, "0")}`,
  );

  let subtitle = $derived(
    discovery.tagsSeen > 0
      ? `${discovery.tagsSeen.toLocaleString()} tags discovered so far…`
      : `Fetching tags from ${registryLabel}/${repository}…`,
  );

  let showLeavePageHint = $derived(discovery.elapsedSeconds >= 15);
</script>

<form onsubmit={onSubmit}>
  <Field
    label="Virtual image name"
    bind:value={name}
    placeholder="myimage"
    required
    disabled={locked}
  />
  <Select
    label="Registry"
    bind:value={registryId}
    options={registryOptions}
    placeholder="Select a registry…"
    required
    disabled={locked}
  />
  <Field
    label="Repository"
    bind:value={repository}
    placeholder="library/nginx"
    required
    disabled={locked}
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
        <p class="discovery-subtitle">{subtitle}</p>
        {#if showLeavePageHint}
          <p class="discovery-hint">
            It's safe to leave this page: discovery keeps running in the
            background.
          </p>
        {/if}
      </div>
      <button
        type="button"
        class="link discovery-cancel"
        onclick={onCancelDiscovery}
      >
        Cancel
      </button>
    </div>
  {/if}

  {#if discovery.error}
    <p class="error">
      <TriangleAlert size={16} strokeWidth={2} />
      {discovery.error}
    </p>
  {/if}

  {#if !discovering}
    <Button type="submit" disabled={inspecting}>
      {inspecting ? "Checking…" : "Check repository"}
    </Button>
  {/if}
</form>

<style>
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
</style>
