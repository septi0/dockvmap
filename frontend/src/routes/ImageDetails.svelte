<script lang="ts">
  import { onDestroy } from "svelte";
  import { link, push } from "svelte-spa-router";
  import Package from "@lucide/svelte/icons/package";
  import ArrowLeft from "@lucide/svelte/icons/arrow-left";
  import ArrowLeftRight from "@lucide/svelte/icons/arrow-left-right";
  import ArrowUp from "@lucide/svelte/icons/arrow-up";
  import Layers from "@lucide/svelte/icons/layers";
  import Plus from "@lucide/svelte/icons/plus";
  import Settings2 from "@lucide/svelte/icons/settings-2";
  import Pin from "@lucide/svelte/icons/pin";
  import PinOff from "@lucide/svelte/icons/pin-off";
  import History from "@lucide/svelte/icons/history";
  import Activity from "@lucide/svelte/icons/activity";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Copy from "@lucide/svelte/icons/copy";
  import Trash2 from "@lucide/svelte/icons/trash-2";
  import AppShell from "../lib/components/AppShell.svelte";
  import PageTitle from "../lib/components/PageTitle.svelte";
  import AsyncState from "../lib/components/AsyncState.svelte";
  import DetailRow from "../lib/components/DetailRow.svelte";
  import Button from "../lib/components/Button.svelte";
  import ChangeTagModal from "../lib/components/ChangeTagModal.svelte";
  import ImageGroupModal from "../lib/components/ImageGroupModal.svelte";
  import TagHistoryModal from "../lib/components/TagHistoryModal.svelte";
  import TagActivityModal from "../lib/components/TagActivityModal.svelte";
  import RefreshTagsButton from "../lib/components/RefreshTagsButton.svelte";
  import RefreshingIndicator from "../lib/components/RefreshingIndicator.svelte";
  import RenameImageModal from "../lib/components/RenameImageModal.svelte";
  import ConfirmDialog from "../lib/components/ConfirmDialog.svelte";
  import TagChangeNotice from "../lib/components/TagChangeNotice.svelte";
  import UpstreamLink from "../lib/components/UpstreamLink.svelte";
  import { getImage, deleteImage, setImagePin, updateImageTag } from "../lib/api/images";
  import { getImageGroup } from "../lib/api/imageGroups";
  import { getPullInfo } from "../lib/api/metrics";
  import { errorMessage } from "../lib/api/client";
  import { copyText } from "../lib/services/clipboard";
  import { toast } from "../lib/services/toast";
  import { updatesCount } from "../lib/services/updatesCount";
  import {
    createImageRefresh,
    applyImageRefreshFields,
  } from "../lib/services/imageRefresh";
  import { allImages, formatDate } from "../lib/utils/format";
  import { pullCommand as buildPullCommand } from "../lib/utils/pullInfo";
  import type { Image } from "../lib/api/types/images";
  import type { ImageGroup } from "../lib/api/types/imageGroups";
  import type { PullInfo } from "../lib/api/types/metrics";

  let { params }: { params: { id: string } } = $props();
  const imageId = $derived(Number(params.id));

  const carriedQuery = window.location.hash.split("?")[1] ?? "";
  const carriedParams = new URLSearchParams(carriedQuery);
  const fromUpdates = carriedParams.get("from") === "updates";
  carriedParams.delete("from");
  const updatesQuery = carriedParams.toString();
  const updatesHref = updatesQuery ? `/updates?${updatesQuery}` : "/updates";
  const imagesHref = carriedQuery ? `/images?${carriedQuery}` : "/images";
  const backHref = fromUpdates ? updatesHref : imagesHref;

  let image = $state<Image | null>(null);
  let group = $state<ImageGroup | null>(null);
  let pullInfo = $state<PullInfo | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let loadToken = 0;

  let showTagModal = $state(false);
  let showGroupModal = $state(false);
  let showHistoryModal = $state(false);
  let showActivityModal = $state(false);

  let copyError = $state<string | null>(null);
  let lastChange = $state<{ from: string; to: string } | null>(null);
  let reverting = $state(false);
  let revertError = $state<string | null>(null);

  let showDeleteConfirm = $state(false);
  let deleting = $state(false);
  let deleteError = $state<string | null>(null);

  let showRenameModal = $state(false);

  let pinning = $state(false);
  let pinError = $state<string | null>(null);

  let refreshStatus = $state<"idle" | "running" | "stale">("idle");
  let modalReloadToken = $state(0);

  let groupSize = $derived(group?.images.length ?? 0);
  let updateAvailable = $derived(
    group ? group.updateAvailable : (image?.updateAvailable ?? false),
  );
  let updateAvailableTag = $derived(
    group ? group.updateAvailableTag : image?.updateAvailableTag,
  );
  let updateBlocked = $derived(!!group && updateAvailable && !updateAvailableTag);
  let pinned = $derived(group ? group.pinned : (image?.pinned ?? false));
  let scopeLabel = $derived(group ? "group" : "image");

  const imageRefresh = createImageRefresh(() => imageId, {
    onResult: (fresh) => {
      if (image) applyImageRefreshFields(image, fresh);
      else image = fresh;
    },
  });

  const stopRefreshSubscription = imageRefresh.subscribe(
    (status) => (refreshStatus = status),
  );
  const stopSettledListener = imageRefresh.onSettled(() => {
    modalReloadToken += 1;
    loadGroup().catch(() => {});
  });

  async function loadGroup() {
    group = image?.group ? await getImageGroup(image.group.id) : null;
  }

  async function load() {
    const requestId = ++loadToken;
    loading = true;
    error = null;
    lastChange = null;
    revertError = null;

    try {
      const [img, info] = await Promise.all([getImage(imageId), getPullInfo()]);
      if (requestId !== loadToken) return;
      image = img;
      pullInfo = info;
      imageRefresh.sync(img);
      await loadGroup();
    } catch (err) {
      if (requestId !== loadToken) return;
      error = errorMessage(err, "Failed to load virtual image");
    } finally {
      if (requestId === loadToken) loading = false;
    }
  }

  $effect(() => {
    imageId;
    imageRefresh.reset();
    load();
  });

  onDestroy(() => {
    stopRefreshSubscription();
    stopSettledListener();
    imageRefresh.destroy();
  });

  const pullCommand = $derived(
    image && pullInfo
      ? buildPullCommand(pullInfo, image.name)
      : "",
  );

  async function refreshImageDetails() {
    try {
      image = await getImage(imageId);
      imageRefresh.sync(image);
      await loadGroup();
    } catch {}
  }

  async function handleTagUpdated(tag: string) {
    lastChange = image ? { from: image.tag, to: tag } : null;
    revertError = null;
    toast.success(
      group
        ? `Tag updated to "${tag}" on ${allImages(groupSize)}.`
        : `Tag updated to "${tag}".`,
    );
    await refreshImageDetails();
  }

  async function revertTag() {
    if (!image || !lastChange) return;

    const { from } = lastChange;

    revertError = null;
    reverting = true;

    try {
      await updateImageTag(image.id, from, "restore");
      updatesCount.refresh();
      await handleTagRestored(from);
    } catch (err) {
      revertError = errorMessage(err, "Failed to revert tag");
    } finally {
      reverting = false;
    }
  }

  async function handleTagRestored(tag: string) {
    lastChange = null;
    toast.success(
      group
        ? `Restored tag to "${tag}" on ${allImages(groupSize)}.`
        : `Restored tag to "${tag}".`,
    );
    await refreshImageDetails();
  }

  async function handleGroupSaved(name: string) {
    toast.success(`Group "${name}" saved.`);
    await refreshImageDetails();
    updatesCount.refresh();
  }

  async function handleGroupDeleted(name: string) {
    toast.success(`Group "${name}" deleted.`);
    await refreshImageDetails();
    updatesCount.refresh();
  }

  async function copyPullCommand() {
    copyError = null;

    if (await copyText(pullCommand)) {
      toast.success("Pull command copied to clipboard.");
    } else {
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

  async function togglePin() {
    if (!image) return;

    const next = !pinned;
    pinError = null;
    pinning = true;

    try {
      await setImagePin(imageId, next);
      await refreshImageDetails();
      toast.success(
        next
          ? `${group ? "Group" : "Image"} pinned. New versions won't be flagged or emailed.`
          : `${group ? "Group" : "Image"} unpinned. Update tracking resumed.`,
      );
    } catch (err) {
      pinError = errorMessage(err, "Failed to update pin state");
    } finally {
      pinning = false;
    }
  }

  async function handleDelete() {
    if (!image) return;

    deleteError = null;
    deleting = true;

    try {
      await deleteImage(imageId);
      toast.success(`Virtual image "${image.name}" deleted.`);
      push(backHref);
    } catch (err) {
      deleteError = errorMessage(err, "Failed to delete virtual image");
    } finally {
      deleting = false;
    }
  }
</script>

<PageTitle title={image ? image.name : "Virtual Image"} />

<AppShell>
  <a class="back-link" href={backHref} use:link>
    <ArrowLeft size={14} strokeWidth={2} />
    {fromUpdates ? "Updates" : "Virtual Images"}
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

      <div class="card section-card details-card">
        <h2>Details</h2>
        <DetailRow label="Registry">{image.registry}</DetailRow>
        <DetailRow label="Repository">
          <span class="detail-cell-row">
            {image.repository}
            <UpstreamLink registry={image.registry} repository={image.repository} />
          </span>
        </DetailRow>
        <DetailRow label="Group">
          <span class="detail-cell-row">
            {#if group}
              <span class="group-name">
                <Layers size={14} strokeWidth={2} />
                {group.name}
              </span>
              <a
                class="group-size"
                href={`/images?group=${group.id}`}
                use:link>{groupSize} images</a
              >
              <Button
                variant="secondary"
                size="sm"
                onclick={() => (showGroupModal = true)}
              >
                <Settings2 size={14} strokeWidth={2} />
                Manage group
              </Button>
            {:else}
              <span class="muted">Not in a group</span>
              <Button
                variant="secondary"
                size="sm"
                onclick={() => (showGroupModal = true)}
              >
                <Plus size={14} strokeWidth={2} />
                Add to group
              </Button>
            {/if}
          </span>
        </DetailRow>
        <DetailRow label="Current tag">
          <div class="detail-cell">
            <span class="detail-cell-row">
              <span class="tag-value">{image.tag}</span>
              {#if updateAvailable && !updateBlocked}
                <Button
                  variant="warning"
                  size="sm"
                  onclick={() => (showTagModal = true)}
                >
                  <ArrowUp size={14} strokeWidth={2.5} />
                  {group ? "Update group" : "Update"}
                </Button>
              {:else}
                <Button
                  variant="secondary"
                  size="sm"
                  onclick={() => (showTagModal = true)}
                >
                  <ArrowLeftRight size={14} strokeWidth={2} />
                  {group ? "Change group tag" : "Change tag"}
                </Button>
              {/if}
            </span>
            {#if lastChange && lastChange.to === image.tag}
              <TagChangeNotice
                from={lastChange.from}
                to={lastChange.to}
                {groupSize}
                disabled={reverting}
                onRevert={revertTag}
              />
            {/if}
            {#if revertError}
              <p class="error">
                <TriangleAlert size={16} strokeWidth={2} />
                {revertError}
              </p>
            {/if}
            {#if updateBlocked}
              <p class="warning-text">
                <TriangleAlert size={14} strokeWidth={2} />
                {#if group?.partialTag}
                  Update blocked: {group.partialTag.tag} is missing from
                  {group.partialTag.missing.join(", ")}.
                {:else}
                  Update blocked: no newer tag is available on {allImages(groupSize)}.
                {/if}
              </p>
            {:else if updateAvailable}
              <p class="warning-text">
                <TriangleAlert size={14} strokeWidth={2} />
                {#if updateAvailableTag}
                  Update available: {image.tag} &rarr; {updateAvailableTag}
                {:else}
                  A newer tag is available in this tag's family.
                {/if}
              </p>
            {/if}
            {#if group}
              <p class="tracking-hint muted">
                Shared by {allImages(groupSize)} in this group.
              </p>
            {/if}
          </div>
        </DetailRow>
        <DetailRow label="Update tracking">
          <div class="detail-cell">
            <span class="detail-cell-row">
              <span class="tracking-state" class:is-pinned={pinned}>
                {#if pinned}
                  <Pin size={14} strokeWidth={2} />
                  Pinned to the current tag
                {:else}
                  Tracking new versions
                {/if}
              </span>
              <Button
                variant="secondary"
                size="sm"
                disabled={pinning}
                onclick={togglePin}
              >
                {#if pinned}
                  <PinOff size={14} strokeWidth={2} />
                  Unpin {scopeLabel}
                {:else}
                  <Pin size={14} strokeWidth={2} />
                  Pin {scopeLabel}
                {/if}
              </Button>
            </span>
            <p class="tracking-hint muted">
              {#if group}
                Pinning applies to {allImages(groupSize)} in this group.
              {:else if image.pinned}
                Upstream tags are still fetched, but newer versions aren't
                flagged as an available update or sent in tag notification
                emails.
              {:else}
                Newer tags in this tag's family are flagged as an available
                update and included in tag notification emails.
              {/if}
            </p>
            {#if pinError}
              <p class="error">
                <TriangleAlert size={16} strokeWidth={2} />
                {pinError}
              </p>
            {/if}
          </div>
        </DetailRow>
        <DetailRow label="Last checked">
          {#if refreshStatus === "stale"}
            <span class="check-stale">
              Couldn't confirm the check finished. Reload to see the result.
            </span>
          {:else if refreshStatus === "running"}
            <RefreshingIndicator text="Checking upstream for tag changes…" />
          {:else}
            {formatDate(image.lastChecked, "Never")}
          {/if}
        </DetailRow>
        {#if image.lastCheckError}
          <DetailRow label="Last check error">
            <div class="check-error-cell">
              <span class="check-error">{image.lastCheckError}</span>
              {#if refreshStatus !== "running"}
                <RefreshTagsButton
                  imageId={image.id}
                  onRefreshed={refreshImageDetails}
                  variant="button"
                />
              {/if}
            </div>
          </DetailRow>
        {/if}
        <DetailRow label="Created">{formatDate(image.createdAt)}</DetailRow>
        <DetailRow label="Updated">{formatDate(image.updatedAt)}</DetailRow>

        <div class="record-actions">
          <Button
            variant="secondary"
            size="sm"
            onclick={() => (showHistoryModal = true)}
          >
            <History size={14} strokeWidth={2} />
            Tag history
          </Button>
          <Button
            variant="secondary"
            size="sm"
            onclick={() => (showActivityModal = true)}
          >
            <Activity size={14} strokeWidth={2} />
            Upstream tag activity
          </Button>
        </div>
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
            class="icon-button bordered"
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
    refreshStatus={refreshStatus === "stale" ? "idle" : refreshStatus}
    reloadToken={modalReloadToken}
    groupName={group?.name}
    {groupSize}
    onClose={() => (showTagModal = false)}
    onTagUpdated={handleTagUpdated}
    onTagsRefreshed={refreshImageDetails}
  />

  <ImageGroupModal
    open={showGroupModal}
    {group}
    {image}
    onClose={() => (showGroupModal = false)}
    onSaved={handleGroupSaved}
    onDeleted={handleGroupDeleted}
  />

  <TagHistoryModal
    open={showHistoryModal}
    imageId={image.id}
    {groupSize}
    onClose={() => (showHistoryModal = false)}
    onRestored={handleTagRestored}
  />

  <TagActivityModal
    open={showActivityModal}
    imageId={image.id}
    onClose={() => (showActivityModal = false)}
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
  message={group
    ? `Delete "${image?.name ?? ""}"? It will be removed from ${group.name}. This cannot be undone.`
    : `Delete "${image?.name ?? ""}"? This cannot be undone.`}
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

  .group-name {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    font-weight: 500;
  }

  .group-size {
    color: var(--color-text-muted);
    font-size: 0.8125rem;
  }

  .group-size:hover {
    color: var(--color-text);
  }

  .tracking-state {
    font-weight: 500;
  }

  .tracking-state.is-pinned {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    font-weight: 600;
    color: var(--color-success);
  }

  .tracking-hint {
    margin: 0;
    font-size: 0.8125rem;
  }

  .details-card :global(.detail-row:has(+ .record-actions)) {
    border-bottom: none;
  }

  .record-actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin-top: var(--space-3);
    padding-top: var(--space-4);
    border-top: 1px solid var(--color-border);
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

  .check-stale {
    font-size: 0.8125rem;
    color: var(--color-text-muted);
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
