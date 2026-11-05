<script lang="ts">
  import { onDestroy, onMount } from "svelte";
  import { link, push } from "svelte-spa-router";
  import Plus from "@lucide/svelte/icons/plus";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Modal from "./Modal.svelte";
  import Button from "./Button.svelte";
  import TagFamilyPicker from "./TagFamilyPicker.svelte";
  import RefreshingIndicator from "./RefreshingIndicator.svelte";
  import { createTagDiscovery } from "../services/tagDiscovery";
  import { writeListQuery } from "../utils/listQuery";
  import type { TagGroup, DiscoveryResult } from "../api/types/images";
  import type { Registry, RegistryRepository } from "../api/types/registries";

  let {
    registry,
    repository,
    onClose,
  }: {
    registry: Registry;
    repository: RegistryRepository | null;
    onClose: () => void;
  } = $props();

  const discovery = createTagDiscovery();

  let tagGroups = $state<TagGroup[]>([]);
  let tagCount = $state(0);
  let ignoredCount = $state(0);
  let selectedTag = $state<string | null>(null);
  let loaded = $state(false);

  onMount(() =>
    discovery.onResolved((result: DiscoveryResult) => {
      tagGroups = result.tagGroups ?? [];
      tagCount = result.tagCount ?? 0;
      ignoredCount = result.ignoredCount ?? 0;
      loaded = true;
    }),
  );
  onDestroy(discovery.destroy);

  $effect(() => {
    if (!repository) return;

    tagGroups = [];
    selectedTag = null;
    loaded = false;
    void discovery.start({ registry: registry.registry, repository: repository.name });
  });

  function close() {
    discovery.cancel();
    onClose();
  }

  function addVirtualImage() {
    if (!repository) return;

    push(
      "/images/new" +
        writeListQuery({ registryId: registry.id, repository: repository.name, tag: selectedTag ?? "" }),
    );
  }
</script>

<Modal open={repository !== null} onClose={close} title={repository?.name} size="lg">
  {#if repository}
    {#if repository.images.length > 0}
      <p class="muted image-links">
        Virtual images:
        {#each repository.images as image, index (image.id)}
          {#if index > 0},
          {/if}<a class="row-link accent" href={`/images/${image.id}`} use:link>{image.name}</a>
        {/each}
      </p>
    {/if}

    {#if $discovery.phase !== "idle"}
      <p class="status">
        <RefreshingIndicator
          text={$discovery.tagsSeen > 0
            ? `Reading tags… ${$discovery.tagsSeen.toLocaleString()} so far`
            : "Reading tags…"}
        />
      </p>
    {:else if $discovery.error}
      <p class="error">
        <TriangleAlert size={16} strokeWidth={2} />
        {$discovery.error}
      </p>
    {:else if loaded}
      <p class="muted summary">
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
        dimPrereleases
        searchable
      />
    {/if}

    <div class="modal-actions spaced">
      <Button variant="secondary" onclick={close}>Close</Button>
      <Button onclick={addVirtualImage}>
        <Plus size={16} strokeWidth={2} />
        Add virtual image
      </Button>
    </div>
  {/if}
</Modal>

<style>
  .image-links {
    margin: 0 0 var(--space-3);
    font-size: 0.875rem;
  }

  .status,
  .summary {
    margin: 0 0 var(--space-3);
    font-size: 0.875rem;
  }
</style>
