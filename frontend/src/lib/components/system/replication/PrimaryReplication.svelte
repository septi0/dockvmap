<script lang="ts">
  import { onMount } from "svelte";
  import Plus from "@lucide/svelte/icons/plus";
  import Trash2 from "@lucide/svelte/icons/trash-2";
  import Button from "../../Button.svelte";
  import CopyButton from "../../CopyButton.svelte";
  import DetailRow from "../../DetailRow.svelte";
  import AsyncState from "../../AsyncState.svelte";
  import StateIndicator from "../../StateIndicator.svelte";
  import ConfirmDialog from "../../ConfirmDialog.svelte";
  import ReplicationTokenModal from "./ReplicationTokenModal.svelte";
  import { listReplicationTokens, deleteReplicationToken } from "../../../api/replication";
  import { errorMessage } from "../../../api/client";
  import { toast } from "../../../services/toast";
  import { formatDate, formatNumber, formatRelativeTime } from "../../../utils/format";
  import { followerState } from "../../../utils/replication";
  import type { PrimaryStatus, ReplicationToken } from "../../../api/types/replication";

  let { status }: { status: PrimaryStatus } = $props();

  let tokens = $state<ReplicationToken[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let showAdd = $state(false);
  let deleting = $state<ReplicationToken | null>(null);
  let deleteError = $state<string | null>(null);
  let submitting = $state(false);

  async function loadTokens() {
    try {
      tokens = await listReplicationTokens();
      error = null;
    } catch (err) {
      error = errorMessage(err, "Failed to load replication tokens");
    } finally {
      loading = false;
    }
  }

  onMount(loadTokens);

  async function handleDelete() {
    if (!deleting) return;

    submitting = true;
    deleteError = null;

    try {
      await deleteReplicationToken(deleting.id);
      toast.success(`Replication token "${deleting.label}" deleted`);
      deleting = null;
      await loadTokens();
    } catch (err) {
      deleteError = errorMessage(err, "Failed to delete the replication token");
    } finally {
      submitting = false;
    }
  }
</script>

<div class="card section-card">
  <h2>Primary</h2>
  <DetailRow label="Replication port">{status.listen}</DetailRow>
  <DetailRow label="Followers allowed from">
    {#if status.allowedIps.length === 0}
      <span class="muted">Anywhere (replication.allowed_ips is empty)</span>
    {:else}
      <span class="mono">{status.allowedIps.join(", ")}</span>
    {/if}
  </DetailRow>
  <DetailRow label="Cluster">{status.cluster}</DetailRow>
  <DetailRow label="State version">{formatNumber(status.stateVersion)}</DetailRow>
  <DetailRow label="Identity">
    <div class="command-row">
      <code class="command">{status.fingerprint}</code>
      <CopyButton text={status.fingerprint} label="Copy fingerprint" copiedMessage="Fingerprint copied to clipboard." bordered />
    </div>
  </DetailRow>
</div>

<div class="card section-card">
  <h2 class="heading-with-count">
    Followers <span class="heading-count">{status.followers.length}</span>
  </h2>
  {#if status.followers.length === 0}
    <p class="muted">No follower connected right now.</p>
  {:else}
    <div class="table-wrap">
      <table class="table">
        <thead>
          <tr>
            <th>Follower</th>
            <th class="phone-hidden">Status</th>
            <th class="phone-hidden">Version</th>
            <th>Last seen</th>
          </tr>
        </thead>
        <tbody>
          {#each status.followers as follower (follower.name + follower.address)}
            {@const state = followerState(follower, status.stateVersion)}
            <tr>
              <td>
                {follower.name}
                <span class="muted address">{follower.address}</span>
                <div class="phone-only follower-status">
                  <StateIndicator tone={state.tone} label={state.label} detail={state.fix} wrap />
                </div>
              </td>
              <td class="phone-hidden">
                <StateIndicator tone={state.tone} label={state.label} detail={state.fix} />
              </td>
              <td class="phone-hidden">{follower.version || "-"}</td>
              <td class="time-cell">
                <span title={formatDate(follower.lastSeen)}>{formatRelativeTime(follower.lastSeen)}</span>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<div class="card section-card">
  <div class="list-header">
    <h2>Replication tokens</h2>
    <Button size="sm" onclick={() => (showAdd = true)}>
      <Plus size={14} strokeWidth={2} />
      Add replication token
    </Button>
  </div>
  <p class="section-hint muted">
    A follower needs one to copy this primary's data, including every registry login. Give each follower its own.
  </p>

  <AsyncState {loading} {error} empty={tokens.length === 0} emptyMessage="No replication tokens yet." emptyVariant="text">

    <div class="table-wrap">
      <table class="table">
        <thead>
          <tr>
            <th>Label</th>
            <th>Created</th>
            <th>Last used</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each tokens as token (token.id)}
            <tr>
              <td>{token.label}</td>
              <td class="time-cell">
                <span title={formatDate(token.createdAt)}>{formatRelativeTime(token.createdAt)}</span>
              </td>
              <td class="time-cell">
                {#if token.lastUsedAt}
                  <span title={formatDate(token.lastUsedAt)}>{formatRelativeTime(token.lastUsedAt)}</span>
                {:else}
                  <span class="muted">-</span>
                {/if}
              </td>
              <td>
                <div class="table-actions">
                  <button
                    type="button"
                    class="icon-button danger"
                    onclick={() => (deleting = token)}
                    aria-label="Delete {token.label}"
                    title="Delete"
                  >
                    <Trash2 size={16} strokeWidth={1.75} />
                  </button>
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </AsyncState>
</div>

<ReplicationTokenModal open={showAdd} listen={status.listen} onClose={() => (showAdd = false)} onCreated={loadTokens} />

<ConfirmDialog
  open={deleting !== null}
  title="Delete replication token"
  message={`Delete "${deleting?.label ?? ""}"? A follower using it stops syncing until it gets a new token. This cannot be undone.`}
  confirmLabel="Delete"
  danger
  error={deleteError}
  {submitting}
  onConfirm={handleDelete}
  onCancel={() => {
    deleting = null;
    deleteError = null;
  }}
/>

<style>
  .address {
    display: block;
    font-size: 0.8125rem;
  }

  .follower-status {
    margin-top: var(--space-1);
  }
</style>
