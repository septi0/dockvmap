<script lang="ts">
  import ArrowLeftRight from "@lucide/svelte/icons/arrow-left-right";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Modal from "./Modal.svelte";
  import Field from "./Field.svelte";
  import Button from "./Button.svelte";
  import { renameImage } from "../api/images";
  import { ApiError } from "../api/client";

  let {
    open,
    imageId,
    currentName,
    virtualTag,
    onClose,
    onRenamed,
  }: {
    open: boolean;
    imageId: number;
    currentName: string;
    virtualTag?: string;
    onClose: () => void;
    onRenamed: (name: string) => void;
  } = $props();

  let newName = $state("");
  let renaming = $state(false);
  let error = $state<string | null>(null);

  const disabled = $derived(
    !newName.trim() || newName.trim() === currentName,
  );

  $effect(() => {
    if (!open) return;

    newName = currentName;
    error = null;
  });

  function guardedClose() {
    if (renaming) return;
    onClose();
  }

  async function handleRename() {
    if (disabled) return;

    error = null;
    renaming = true;

    try {
      const trimmed = newName.trim();
      await renameImage(imageId, trimmed);
      onRenamed(trimmed);
      onClose();
    } catch (err) {
      error = err instanceof ApiError ? err.message : "Failed to rename virtual image";
    } finally {
      renaming = false;
    }
  }
</script>

<Modal {open} onClose={guardedClose} title="Rename virtual image">
  <p class="hint muted">
    Renaming changes the path clients pull from. Any client currently pulling
    <code class="inline-code">{currentName}{virtualTag ? `:${virtualTag}` : ""}</code>
    will stop working immediately once renamed.
  </p>

  <Field label="New name" bind:value={newName} disabled={renaming} />

  {#if error}
    <p class="error">
      <TriangleAlert size={16} strokeWidth={2} />
      {error}
    </p>
  {/if}

  <div class="actions">
    <Button variant="secondary" onclick={guardedClose} disabled={renaming}
      >Cancel</Button
    >
    <Button variant="secondary" onclick={handleRename} disabled={disabled || renaming}>
      <ArrowLeftRight size={16} strokeWidth={1.75} />
      {renaming ? "Renaming…" : "Rename"}
    </Button>
  </div>
</Modal>

<style>
  .hint {
    margin: 0 0 var(--space-4);
    font-size: 0.875rem;
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
    margin-top: var(--space-4);
  }
</style>
