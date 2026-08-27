<script lang="ts">
  import { link, push } from "svelte-spa-router";
  import Package from "@lucide/svelte/icons/package";
  import ArrowLeft from "@lucide/svelte/icons/arrow-left";
  import ArrowLeftRight from "@lucide/svelte/icons/arrow-left-right";
  import ArrowUp from "@lucide/svelte/icons/arrow-up";
  import History from "@lucide/svelte/icons/history";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Copy from "@lucide/svelte/icons/copy";
  import Trash2 from "@lucide/svelte/icons/trash-2";
  import AppShell from "../lib/components/AppShell.svelte";
  import AsyncState from "../lib/components/AsyncState.svelte";
  import DetailRow from "../lib/components/DetailRow.svelte";
  import Button from "../lib/components/Button.svelte";
  import ChangeTagModal from "../lib/components/ChangeTagModal.svelte";
  import TagHistoryModal from "../lib/components/TagHistoryModal.svelte";
  import RefreshTagsButton from "../lib/components/RefreshTagsButton.svelte";
  import RenameImageModal from "../lib/components/RenameImageModal.svelte";
  import ConfirmDialog from "../lib/components/ConfirmDialog.svelte";
  import { getImage, deleteImage } from "../lib/api/images";
  import { getPullInfo } from "../lib/api/metrics";
  import { ApiError } from "../lib/api/client";
  import { toast } from "../lib/services/toast";
  import { formatDate } from "../lib/utils/format";
  import type { Image } from "../lib/api/types/images";
  import type { PullInfo } from "../lib/api/types/metrics";

  let { params }: { params: { id: string } } = $props();
  const imageId = $derived(Number(params.id));

  let image = $state<Image | null>(null);
  let pullInfo = $state<PullInfo | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let loadToken = 0;

  let showTagModal = $state(false);
  let showHistoryModal = $state(false);

  let copyError = $state<string | null>(null);

  let showDeleteConfirm = $state(false);
  let deleting = $state(false);
  let deleteError = $state<string | null>(null);

  let showRenameModal = $state(false);

  async function load() {
    const requestId = ++loadToken;
    loading = true;
    error = null;

    try {
      const [img, info] = await Promise.all([getImage(imageId), getPullInfo()]);
      if (requestId !== loadToken) return;
      image = img;
      pullInfo = info;
    } catch (err) {
      if (requestId !== loadToken) return;
      error =
        err instanceof ApiError ? err.message : "Failed to load virtual image";
    } finally {
      if (requestId === loadToken) loading = false;
    }
  }

  $effect(() => {
    imageId;
    load();
  });

  const pullCommand = $derived(
    image && pullInfo
      ? `docker pull ${pullInfo.host}:${pullInfo.port}/${image.name}:${pullInfo.virtualTag}`
      : "",
  );

  async function refreshImageDetails() {
    try {
      image = await getImage(imageId);
    } catch {}
  }

  async function handleTagUpdated(tag: string) {
    toast.success(`Tag updated to "${tag}".`);
    await refreshImageDetails();
  }

  async function handleTagRestored(tag: string) {
    toast.success(`Restored tag to "${tag}".`);
    await refreshImageDetails();
  }

  async function copyPullCommand() {
    copyError = null;

    try {
      await navigator.clipboard.writeText(pullCommand);
      toast.success("Pull command copied to clipboard.");
    } catch {
      copyError =
        "Could not copy automatically - select and copy the command manually.";
    }
  }

  function cancelDelete() {
    showDeleteConfirm = false;
    deleteError = null;
  }

  async function handleRenamed(name: string) {
    toast.success(`Virtual image renamed to "${name}".`);
    await refreshImageDetails();
  }

  async function handleDelete() {
    if (!image) return;

    deleteError = null;
    deleting = true;

    try {
      await deleteImage(imageId);
      toast.success(`Virtual image "${image.name}" deleted.`);
      push("/images");
    } catch (err) {
      deleteError =
        err instanceof ApiError
          ? err.message
          : "Failed to delete virtual image";
    } finally {
      deleting = false;
    }
  }
</script>

<AppShell>
  <a class="back-link" href="/images" use:link>
    <ArrowLeft size={14} strokeWidth={2} />
    Virtual Images
  </a>

  <AsyncState {loading} {error}>
    {#if image}
      <div class="page-header">
        <div class="title-row">
          <Package size={20} strokeWidth={1.75} />
          <h1>{image.name}</h1>
        </div>
        <p class="subtitle">
          <strong>{image.registry}</strong>/{image.repository}
        </p>
      </div>

      <div class="card section-card">
        <h2>Details</h2>
        <DetailRow label="Registry">{image.registry}</DetailRow>
        <DetailRow label="Repository">{image.repository}</DetailRow>
        <DetailRow label="Current tag">
          <div class="detail-cell">
            <span class="detail-cell-row">
              <span class="tag-value">{image.tag}</span>
              {#if image.updateAvailable}
                <Button
                  variant="warning"
                  size="sm"
                  onclick={() => (showTagModal = true)}
                >
                  <ArrowUp size={14} strokeWidth={2.5} />
                  Update
                </Button>
              {:else}
                <Button
                  variant="secondary"
                  size="sm"
                  onclick={() => (showTagModal = true)}
                >
                  <ArrowLeftRight size={14} strokeWidth={2} />
                  Change tag
                </Button>
              {/if}
              <Button
                variant="secondary"
                size="sm"
                onclick={() => (showHistoryModal = true)}
              >
                <History size={14} strokeWidth={2} />
                History
              </Button>
            </span>
            {#if image.updateAvailable}
              <p class="warning-text">
                <TriangleAlert size={14} strokeWidth={2} />
                {#if image.updateAvailableTag}
                  Update available: {image.tag} &rarr; {image.updateAvailableTag}
                {:else}
                  A newer tag is available in this tag's family.
                {/if}
              </p>
            {/if}
          </div>
        </DetailRow>
        <DetailRow label="Last checked"
          >{formatDate(image.lastChecked, "Never")}</DetailRow
        >
        {#if image.lastCheckError}
          <DetailRow label="Last check error">
            <div class="check-error-cell">
              <span class="check-error">{image.lastCheckError}</span>
              <RefreshTagsButton
                imageId={image.id}
                onRefreshed={refreshImageDetails}
                variant="button"
              />
            </div>
          </DetailRow>
        {/if}
        <DetailRow label="Created">{formatDate(image.createdAt)}</DetailRow>
        <DetailRow label="Updated">{formatDate(image.updatedAt)}</DetailRow>
      </div>

      <div class="card section-card">
        <h2>Pull command</h2>
        <p class="section-hint muted">
          Clients pull this virtual image using a stable tag that always
          resolves to the version you choose above.
        </p>

        <p class="field-hint muted">
          {#if pullInfo?.hostConfigured}
            Host pinned via the <code class="inline-code"
              >proxy_public_host</code
            > config option.
          {:else}
            Host based on the address used to reach this page. Set <code
              class="inline-code">proxy_public_host</code
            > in config if clients reach the registry through a different hostname.
          {/if}
        </p>

        <div class="command-row">
          <code class="command">{pullCommand}</code>
          <button
            type="button"
            class="icon-button"
            onclick={copyPullCommand}
            aria-label="Copy pull command"
          >
            <Copy size={16} strokeWidth={1.75} />
          </button>
        </div>

        {#if copyError}
          <p class="error">
            <TriangleAlert size={16} strokeWidth={2} />
            {copyError}
          </p>
        {/if}
      </div>

      <div class="card section-card danger-card">
        <h2>Danger zone</h2>

        <div class="danger-action">
          <p class="section-hint muted">
            Renaming changes the path clients pull from. Any client currently
            pulling <code class="inline-code"
              >{image.name}:{pullInfo?.virtualTag}</code
            > will stop working immediately once renamed.
          </p>
          <Button variant="secondary" onclick={() => (showRenameModal = true)}>
            <ArrowLeftRight size={16} strokeWidth={1.75} />
            Rename virtual image
          </Button>
        </div>

        <div class="danger-action">
          <p class="section-hint muted">
            Deleting this virtual image stops the proxy from resolving
            <code class="inline-code">{image.name}:{pullInfo?.virtualTag}</code>
            and removes its tracked tag history. This cannot be undone.
          </p>
          <Button variant="danger" onclick={() => (showDeleteConfirm = true)}>
            <Trash2 size={16} strokeWidth={1.75} />
            Delete virtual image
          </Button>
        </div>
      </div>
    {/if}
  </AsyncState>
</AppShell>

{#if image}
  <ChangeTagModal
    open={showTagModal}
    imageId={image.id}
    currentTag={image.tag}
    onClose={() => (showTagModal = false)}
    onTagUpdated={handleTagUpdated}
    onTagsRefreshed={refreshImageDetails}
  />

  <TagHistoryModal
    open={showHistoryModal}
    imageId={image.id}
    onClose={() => (showHistoryModal = false)}
    onRestored={handleTagRestored}
  />

  <RenameImageModal
    open={showRenameModal}
    imageId={image.id}
    currentName={image.name}
    virtualTag={pullInfo?.virtualTag}
    onClose={() => (showRenameModal = false)}
    onRenamed={handleRenamed}
  />
{/if}

<ConfirmDialog
  open={showDeleteConfirm}
  title="Delete virtual image"
  message={`Delete "${image?.name ?? ""}"? This cannot be undone.`}
  confirmLabel="Delete"
  danger
  error={deleteError}
  submitting={deleting}
  onConfirm={handleDelete}
  onCancel={cancelDelete}
/>


<style>
  .back-link {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    margin-bottom: var(--space-4);
    font-size: 0.8125rem;
    color: var(--color-text-muted);
    text-decoration: none;
  }

  .back-link:hover {
    color: var(--color-text);
  }

  .field-hint {
    margin: var(--space-1) 0 var(--space-4);
    font-size: 0.8125rem;
  }

  .command-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .command {
    flex: 1;
    min-width: 0;
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    background: var(--color-bg);
    border: 1px solid var(--color-border);
    font-family: ui-monospace, monospace;
    font-size: 0.8125rem;
    overflow-x: auto;
    white-space: pre;
  }

  .icon-button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    padding: var(--space-2);
    border: 1px solid var(--color-border-strong);
    background: var(--color-surface);
    border-radius: var(--radius-md);
    color: var(--color-text-muted);
    cursor: pointer;
  }

  .icon-button:hover {
    background: var(--color-surface-hover);
    color: var(--color-text);
  }

  .check-error-cell {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-2);
  }

  .check-error {
    color: var(--color-danger);
    word-break: break-word;
  }

  .danger-card {
    margin-bottom: 0;
    border-color: color-mix(in srgb, var(--color-danger) 40%, transparent);
  }

  .danger-action {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-2);
  }

  .danger-action + .danger-action {
    margin-top: var(--space-4);
    padding-top: var(--space-4);
    border-top: 1px solid var(--color-border);
  }
</style>
