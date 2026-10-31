<script lang="ts">
  import Check from "@lucide/svelte/icons/check";
  import RotateCcw from "@lucide/svelte/icons/rotate-ccw";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Modal from "./Modal.svelte";
  import Button from "./Button.svelte";
  import AsyncState from "./AsyncState.svelte";
  import { getTagHistory, getImageTags, updateImageTag } from "../api/images";
  import { errorMessage } from "../api/client";
  import { allImages } from "../utils/format";
  import { formatDate } from "../utils/format";
  import type { TagHistoryEntry } from "../api/types/images";

  let {
    open,
    imageId,
    groupSize = 0,
    onClose,
    onRestored,
  }: {
    open: boolean;
    imageId: number;
    groupSize?: number;
    onClose: () => void;
    onRestored: (tag: string) => void;
  } = $props();

  let history = $state<TagHistoryEntry[]>([]);
  let availableTags = $state<Set<string>>(new Set());
  let loading = $state(true);
  let loadError = $state<string | null>(null);

  let restoring = $state<number | null>(null);
  let restoreError = $state<string | null>(null);
  let loadToken = 0;

  async function load() {
    const requestId = ++loadToken;
    loading = true;
    loadError = null;

    try {
      const [historyResult, tagsResult] = await Promise.all([
        getTagHistory(imageId),
        getImageTags(imageId),
      ]);
      if (requestId !== loadToken) return;
      history = historyResult.history;
      availableTags = new Set(
        tagsResult.tagGroups.flatMap((group) => group.tags.map((t) => t.tag)),
      );
    } catch (err) {
      if (requestId !== loadToken) return;
      loadError = errorMessage(err, "Failed to load tag history");
    } finally {
      if (requestId === loadToken) loading = false;
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
      restoreError = errorMessage(err, "Failed to restore tag");
    } finally {
      restoring = null;
    }
  }
</script>

<Modal {open} {onClose} title="Tag history" size="lg">
  <p class="section-hint muted">
    Every tag this virtual image has pointed to, most recent first.
    {#if groupSize > 0}
      Restoring a tag changes {allImages(groupSize)} in the group.
    {/if}
  </p>

  <AsyncState
    {loading}
    error={loadError}
    empty={history.length === 0}
    emptyVariant="text"
    emptyMessage="No history yet."
    listSkeleton
  >
    <ul class="entry-list">
      {#each history as entry, index (entry.id)}
        {@const isCurrent = index === 0}
        {@const isAvailable = availableTags.has(entry.tag)}
        <li class="entry-card" class:current={isCurrent}>
          <div class="entry-main">
            <span class="history-tag">{entry.tag}</span>
            {#if isCurrent}
              <span class="current-badge">
                <Check size={12} strokeWidth={3} /> Current
              </span>
            {/if}
          </div>
          <div class="entry-meta">
            <span class="history-source">{sourceLabel(entry)}</span>
            <span class="entry-date">{formatDate(entry.appliedAt)}</span>
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
  </AsyncState>

  {#if restoreError}
    <p class="error">
      <TriangleAlert size={16} strokeWidth={2} />
      {restoreError}
    </p>
  {/if}
</Modal>

<style>
  .entry-card.current {
    border-color: var(--color-accent);
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
