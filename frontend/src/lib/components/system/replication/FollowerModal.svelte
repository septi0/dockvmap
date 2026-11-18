<script lang="ts">
  import { untrack } from "svelte";
  import Trash2 from "@lucide/svelte/icons/trash-2";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Modal from "../../Modal.svelte";
  import Button from "../../Button.svelte";
  import Field from "../../Field.svelte";
  import DetailRow from "../../DetailRow.svelte";
  import StateIndicator from "../../StateIndicator.svelte";
  import ConfirmDialog from "../../ConfirmDialog.svelte";
  import { removeReplicationFollower, renameReplicationFollower } from "../../../api/replication";
  import { errorMessage } from "../../../api/client";
  import { toast } from "../../../services/toast";
  import { formatCount, formatDate, formatNumber, formatRelativeTime } from "../../../utils/format";
  import { followerState } from "../../../utils/replication";
  import type { SeenFollower } from "../../../api/types/replication";

  let {
    follower,
    stateVersion,
    onClose,
    onChanged,
  }: {
    follower: SeenFollower | null;
    stateVersion: number;
    onClose: () => void;
    onChanged: () => void;
  } = $props();

  let nameInput = $state("");
  let editingId = $state<number | null>(null);
  let saving = $state(false);
  let saveError = $state<string | null>(null);
  let confirming = $state(false);
  let removing = $state(false);
  let removeError = $state<string | null>(null);

  let view = $derived(follower ? followerState(follower, stateVersion) : null);
  let title = $derived(follower ? follower.name || `Follower ${follower.address}` : "Follower");
  let health = $derived(follower?.connection === "connected" ? follower.health : undefined);
  let unchanged = $derived(nameInput.trim() === (follower?.name ?? ""));

  // the page refreshes every few seconds, so the field is filled only when another follower opens
  $effect(() => {
    const id = follower?.id ?? null;

    untrack(() => {
      if (id === editingId) return;

      editingId = id;
      nameInput = follower?.name ?? "";
      saveError = null;
    });
  });

  function copies(pending: number, failed: number): string {
    if (pending === 0 && failed === 0) return "Nothing to copy";

    const parts = [];

    if (pending > 0) parts.push(`${formatCount(pending, "repository", "repositories")} to copy`);
    if (failed > 0) parts.push(`${formatNumber(failed)} failing`);

    return parts.join(", ");
  }

  async function handleSave(event: SubmitEvent) {
    event.preventDefault();

    if (!follower) return;

    saving = true;
    saveError = null;

    try {
      await renameReplicationFollower(follower.id, nameInput);
      toast.success(nameInput.trim() ? `Follower renamed to "${nameInput.trim()}"` : "Follower name cleared");
      onChanged();
    } catch (err) {
      saveError = errorMessage(err, "Failed to rename the follower");
    } finally {
      saving = false;
    }
  }

  async function handleRemove() {
    if (!follower) return;

    removing = true;
    removeError = null;

    try {
      await removeReplicationFollower(follower.id);
      toast.success(`${title} removed`);
      confirming = false;
      onChanged();
    } catch (err) {
      removeError = errorMessage(err, "Failed to remove the follower");
    } finally {
      removing = false;
    }
  }
</script>

<Modal open={follower !== null} {onClose} {title}>
  {#if follower && view}
    <form onsubmit={handleSave}>
      {#if saveError}
        <p class="error">
          <TriangleAlert size={16} strokeWidth={2} />
          {saveError}
        </p>
      {/if}

      <div>
        <Field label="Name" bind:value={nameInput} placeholder={follower.address} />
        <p class="section-hint muted name-hint">Shown instead of its IP. Leave empty to show the IP.</p>
      </div>

      <div>
        <DetailRow label="Status">
          <StateIndicator tone={view.tone} label={view.label} detail={view.fix} wrap />
        </DetailRow>
        <DetailRow label="IP"><span class="mono">{follower.address}</span></DetailRow>
        <DetailRow label="DockVMap version">{follower.version || "-"}</DetailRow>
        <DetailRow label="Replication token">
          {#if follower.token}
            {follower.token}
          {:else}
            <span class="muted">Deleted; this follower needs a new token to sync</span>
          {/if}
        </DetailRow>

        {#if follower.connection === "connected"}
          <DetailRow label="State version">
            {formatNumber(follower.appliedVersion)} <span class="muted">(this primary: {formatNumber(stateVersion)})</span>
          </DetailRow>
        {/if}

        {#if health}
          <DetailRow label="Ready to take over">
            {health.ready ? "Yes" : `No: ${health.notReady}`}
          </DetailRow>
          <DetailRow label="Registry copies">{copies(health.pendingRepositories, health.failedCopies)}</DetailRow>
          <DetailRow label="Tags the primary doesn't have">
            {#if health.extraTags === undefined}
              <span class="muted">Not checked since it started; its daily reconcile counts them</span>
            {:else if health.extraTags === 0}
              None
            {:else}
              {formatNumber(health.extraTags)}
              <span class="muted">Run <code class="inline-code">dockvmap -resync</code> on the follower to remove them.</span>
            {/if}
          </DetailRow>
          {#if health.copyError}
            <DetailRow label="Copy error"><span class="error-text">{health.copyError}</span></DetailRow>
          {/if}
          {#if health.lastError}
            <DetailRow label="Sync error"><span class="error-text">{health.lastError}</span></DetailRow>
          {/if}
        {:else if follower.connection === "connected"}
          <p class="muted notice-text">This follower runs an older DockVMap that doesn't report its copies. Update it to see them.</p>
        {:else}
          <p class="muted notice-text">Its copies and errors show while it's connected.</p>
        {/if}

        <DetailRow label="Last seen">
          <span title={formatDate(follower.lastSeen)}>{formatRelativeTime(follower.lastSeen)}</span>
        </DetailRow>
        <DetailRow label="First seen">
          <span title={formatDate(follower.firstSeen)}>{formatRelativeTime(follower.firstSeen)}</span>
        </DetailRow>
      </div>

      <div class="modal-actions split">
        <div class="modal-actions-left">
          <Button type="button" variant="danger" onclick={() => (confirming = true)}>
            <Trash2 size={16} strokeWidth={1.75} />
            Remove
          </Button>
        </div>
        <div class="modal-actions-right">
          <Button type="button" variant="secondary" onclick={onClose}>Cancel</Button>
          <Button type="submit" disabled={saving || unchanged}>{saving ? "Saving…" : "Save changes"}</Button>
        </div>
      </div>
    </form>
  {/if}
</Modal>

<ConfirmDialog
  open={confirming}
  title="Remove follower"
  message={`Remove ${title} from this list? If it still runs, it shows up again when it next connects. To stop it syncing, delete its replication token.`}
  confirmLabel="Remove"
  danger
  error={removeError}
  submitting={removing}
  onConfirm={handleRemove}
  onCancel={() => {
    confirming = false;
    removeError = null;
  }}
/>

<style>
  .name-hint {
    margin: var(--space-1) 0 0;
  }

  .notice-text {
    margin: var(--space-3) 0;
    font-size: 0.875rem;
  }

  .error-text {
    overflow-wrap: anywhere;
    color: var(--color-danger);
  }
</style>
