<script lang="ts">
  import Check from "@lucide/svelte/icons/check";
  import RotateCcw from "@lucide/svelte/icons/rotate-ccw";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Modal from "./Modal.svelte";
  import Button from "./Button.svelte";
  import { getTagHistory, getImageTags, updateImageTag } from "../api/images";
  import { ApiError } from "../api/client";
  import { formatDate } from "../utils/format";
  import type { TagHistoryEntry } from "../api/types/images";

  let {
    open,
    imageId,
    onClose,
    onRestored,
  }: {
    open: boolean;
    imageId: number;
    onClose: () => void;
    onRestored: (tag: string) => void;
  } = $props();

  let history = $state<TagHistoryEntry[]>([]);
  let availableTags = $state<Set<string>>(new Set());
  let loading = $state(false);
  let loadError = $state<string | null>(null);

  let restoring = $state<number | null>(null);
  let restoreError = $state<string | null>(null);

  async function load() {
    loading = true;
    loadError = null;

    try {
      const [historyResult, tagsResult] = await Promise.all([
        getTagHistory(imageId),
        getImageTags(imageId),
      ]);
      history = historyResult.history;
      availableTags = new Set(
        tagsResult.tagGroups.flatMap((group) => group.tags.map((t) => t.tag)),
      );
    } catch (err) {
      loadError =
        err instanceof ApiError ? err.message : "Failed to load tag history";
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    if (!open) return;

    restoreError = null;
    load();
  });

  function sourceLabel(entry: TagHistoryEntry): string {
    if (entry.source === "created") return "Created";

    const verb = entry.source === "restore" ? "Restored" : "Changed";

    return entry.previousTag ? `${verb} from ${entry.previousTag}` : verb;
  }

  async function handleRestore(entry: TagHistoryEntry) {
    restoreError = null;
    restoring = entry.id;

    try {
      await updateImageTag(imageId, entry.tag, "restore");
      onRestored(entry.tag);
      onClose();
    } catch (err) {
      restoreError =
        err instanceof ApiError ? err.message : "Failed to restore tag";
    } finally {
      restoring = null;
    }
  }
</script>

<Modal {open} {onClose} title="Tag history" size="lg">
  <p class="hint muted">
    Every tag this virtual image has pointed to, most recent first.
  </p>

  {#if loadError}
    <p class="error">
      <TriangleAlert size={16} strokeWidth={2} />
      {loadError}
    </p>
  {:else if loading}
    <p class="muted">Loading…</p>
  {:else if history.length === 0}
    <p class="muted">No history yet.</p>
  {:else}
    <ul class="history-list">
      {#each history as entry, index (entry.id)}
        {@const isCurrent = index === 0}
        {@const isAvailable = availableTags.has(entry.tag)}
        <li class="history-entry" class:current={isCurrent}>
          <div class="history-main">
            <span class="history-tag">{entry.tag}</span>
            {#if isCurrent}
              <span class="current-badge">
                <Check size={12} strokeWidth={3} /> Current
              </span>
            {/if}
          </div>
          <div class="history-meta">
            <span class="history-source">{sourceLabel(entry)}</span>
            <span class="history-date">{formatDate(entry.appliedAt)}</span>
          </div>
          {#if !isCurrent}
            <div class="history-action">
              <Button
                variant="secondary"
                size="sm"
                disabled={!isAvailable || restoring !== null}
                onclick={() => handleRestore(entry)}
              >
                <RotateCcw size={14} strokeWidth={2} />
                {restoring === entry.id ? "Restoring…" : "Restore"}
              </Button>
              {#if !isAvailable}
                <span class="unavailable-note muted"
                  >No longer available upstream</span
                >
              {/if}
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}

  {#if restoreError}
    <p class="error">
      <TriangleAlert size={16} strokeWidth={2} />
      {restoreError}
    </p>
  {/if}
</Modal>

<style>
  .hint {
    margin: 0 0 var(--space-4);
    font-size: 0.875rem;
  }

  .history-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .history-entry {
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
  }

  .history-entry.current {
    border-color: var(--color-accent);
  }

  .history-main {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .history-tag {
    font-family: monospace;
    font-size: 0.9375rem;
    font-weight: 600;
    color: var(--color-text);
  }

  .current-badge {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    padding: 1px var(--space-2);
    border-radius: var(--radius-full);
    background: var(--color-accent-muted-bg);
    color: var(--color-accent);
    font-size: 0.75rem;
    font-weight: 600;
  }

  .history-meta {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-top: var(--space-1);
    color: var(--color-text-muted);
    font-size: 0.8125rem;
  }

  .history-date::before {
    content: "\2022";
    margin-right: var(--space-2);
    color: var(--color-text-faint);
  }

  .history-action {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin-top: var(--space-3);
  }

  .unavailable-note {
    font-size: 0.75rem;
  }
</style>
