<script lang="ts">
  import { onDestroy } from "svelte";
  import { link, push } from "svelte-spa-router";
  import Package from "@lucide/svelte/icons/package";
  import ArrowLeft from "@lucide/svelte/icons/arrow-left";
  import ArrowLeftRight from "@lucide/svelte/icons/arrow-left-right";
  import History from "@lucide/svelte/icons/history";
  import Activity from "@lucide/svelte/icons/activity";
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
  import UpstreamLink from "../lib/components/UpstreamLink.svelte";
  import GroupRow from "../lib/components/imageDetails/GroupRow.svelte";
  import CurrentTagRow from "../lib/components/imageDetails/CurrentTagRow.svelte";
  import UpdateTrackingRow from "../lib/components/imageDetails/UpdateTrackingRow.svelte";
  import PullCommand from "../lib/components/imageDetails/PullCommand.svelte";
  import RunningOnCard from "../lib/components/imageDetails/RunningOnCard.svelte";
  import PullActivityRow from "../lib/components/imageDetails/PullActivityRow.svelte";
  import { getImage, deleteImage, setImagePin, updateImageTag } from "../lib/api/images";
  import { getImageGroup } from "../lib/api/imageGroups";
  import { getPullInfo } from "../lib/api/metrics";
  import { errorMessage } from "../lib/api/client";
  import { toast } from "../lib/services/toast";
  import { updatesCount } from "../lib/services/updatesCount";
  import {
    createImageRefresh,
    applyImageRefreshFields,
  } from "../lib/services/imageRefresh";
  import {
    allImages,
    formatDate,
    formatRelativeTime,
    shortDigest,
  } from "../lib/utils/format";
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

  const DIGEST_LOOKUP_WAIT_MS = 2000;
  let digestRefreshTimer: ReturnType<typeof setTimeout> | undefined;

  let groupSize = $derived(group?.images.length ?? 0);
  let pinned = $derived(group ? group.pinned : (image?.pinned ?? false));

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
    clearTimeout(digestRefreshTimer);
  });

  async function refreshImageDetails() {
    try {
      image = await getImage(imageId);
      imageRefresh.sync(image);
      await loadGroup();
    } catch {}
  }

  async function refreshAfterTagChange() {
    await refreshImageDetails();

    if (!image?.currentDigest) {
      clearTimeout(digestRefreshTimer);
      digestRefreshTimer = setTimeout(refreshImageDetails, DIGEST_LOOKUP_WAIT_MS);
    }
  }

  async function handleTagUpdated(tag: string) {
    lastChange = image ? { from: image.tag, to: tag } : null;
    revertError = null;
    toast.success(
      group
        ? `Tag updated to "${tag}" on ${allImages(groupSize)}.`
        : `Tag updated to "${tag}".`,
    );
    await refreshAfterTagChange();
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
      revertError = errorMessage(err, "Failed to roll back");
    } finally {
      reverting = false;
    }
  }

  async function handleTagRestored(tag: string) {
    lastChange = null;
    toast.success(
      group
        ? `Rolled back to "${tag}" on ${allImages(groupSize)}.`
        : `Rolled back to "${tag}".`,
    );
    await refreshAfterTagChange();
  }

  async function handleGroupSaved(name: string) {
    toast.success(`Group "${name}" saved.`);
    await refreshAfterTagChange();
    updatesCount.refresh();
  }

  async function handleGroupDeleted(name: string) {
    toast.success(`Group "${name}" deleted.`);
    await refreshImageDetails();
    updatesCount.refresh();
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
      <div class="list-header detail-header">
        <div>
          <div class="title-row">
            <Package size={20} strokeWidth={1.75} />
            <h1>{image.name}</h1>
          </div>
          <p class="subtitle image-source">
            <span><strong>{image.registry}</strong>/{image.repository}</span>
            <UpstreamLink registry={image.registry} repository={image.repository} />
          </p>
        </div>
        <div class="header-actions">
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

      <PullCommand {pullInfo} imageName={image.name} />

      <div class="card section-card">
        <h2>Version</h2>
        <CurrentTagRow
          {image}
          {group}
          {groupSize}
          {lastChange}
          {reverting}
          {revertError}
          onChangeTag={() => (showTagModal = true)}
          onRevert={revertTag}
        />
        <DetailRow label="Digest">
          {#if image.currentDigest}
            <span class="detail-cell-row">
              <span class="digest" title={image.currentDigest}
                >{shortDigest(image.currentDigest)}</span
              >
              {#if image.currentDigestSince}
                <span
                  class="muted"
                  title={formatDate(image.currentDigestSince)}
                  >first seen {formatRelativeTime(image.currentDigestSince)}</span
                >
              {/if}
            </span>
          {:else}
            <span
              class="muted"
              title="The registry didn't report a digest for this tag, or it hasn't been looked up yet."
              >Unknown</span
            >
          {/if}
        </DetailRow>
        <UpdateTrackingRow
          {group}
          {groupSize}
          {pinned}
          {pinning}
          {pinError}
          onTogglePin={togglePin}
        />
        <GroupRow
          {group}
          {groupSize}
          onManage={() => (showGroupModal = true)}
        />
      </div>

      <div class="card section-card">
        <h2>Activity</h2>
        <DetailRow label="Last checked">
          {#if refreshStatus === "stale"}
            <span class="check-stale">
              Couldn't confirm the check finished. Reload to see the result.
            </span>
          {:else if refreshStatus === "running"}
            <RefreshingIndicator text="Checking upstream for tag changes…" />
          {:else}
            <span title={image.lastChecked ? formatDate(image.lastChecked) : undefined}
              >{formatRelativeTime(image.lastChecked, "Never")}</span
            >
          {/if}
        </DetailRow>
        <PullActivityRow {image} />
        {#if image.lastCheckError}
          <DetailRow label="Last check error">
            <div class="check-error-cell">
              <span class="check-error">{image.lastCheckError}</span>
              {#if refreshStatus !== "running"}
                <RefreshTagsButton
                  imageId={image.id}
                  onRefreshed={refreshImageDetails}
                />
              {/if}
            </div>
          </DetailRow>
        {/if}
        <p class="record-dates muted">
          Created <span title={formatDate(image.createdAt)}
            >{formatRelativeTime(image.createdAt)}</span
          >
          · updated <span title={formatDate(image.updatedAt)}
            >{formatRelativeTime(image.updatedAt)}</span
          >
        </p>
      </div>

      <RunningOnCard
        imageId={image.id}
        refreshKey={`${image.tag}:${image.currentDigest ?? ""}`}
      />

      <div class="card section-card danger-card">
        <h2>Danger zone</h2>

        <div class="danger-action">
          <div class="danger-text">
            <strong>Rename virtual image</strong>
            <p class="muted">
              Changes the path clients pull from;
              <code class="inline-code">{image.name}:{pullInfo?.virtualTag}</code>
              stops working right away.
            </p>
          </div>
          <Button variant="secondary" size="sm" onclick={() => (showRenameModal = true)}>
            <ArrowLeftRight size={14} strokeWidth={2} />
            Rename
          </Button>
        </div>

        <div class="danger-action">
          <div class="danger-text">
            <strong>Delete virtual image</strong>
            <p class="muted">
              The proxy stops resolving
              <code class="inline-code">{image.name}:{pullInfo?.virtualTag}</code>
              and its tag history is removed. This cannot be undone.
            </p>
          </div>
          <Button variant="danger" size="sm" onclick={() => (showDeleteConfirm = true)}>
            <Trash2 size={14} strokeWidth={2} />
            Delete
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
  .image-source {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1) var(--space-3);
  }

  .record-dates {
    margin: var(--space-3) 0 0;
    font-size: 0.8125rem;
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

  .digest {
    font-family: monospace;
  }
</style>
