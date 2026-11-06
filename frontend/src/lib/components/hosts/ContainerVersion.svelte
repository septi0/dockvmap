<script lang="ts">
  import StateIndicator from "../StateIndicator.svelte";
  import type { ContainerTracking } from "../../api/types/hosts";

  let { tracking, showCurrent = false }: { tracking: ContainerTracking; showCurrent?: boolean } = $props();

  let target = $derived(tracking.tag ?? "the new image");
</script>

{#if tracking.needsUpdate}
  <StateIndicator
    tone="warning"
    wrap
    label="Needs a redeploy to run {target}"
    title={tracking.status === "pulledNotRecreated"
      ? `${target} is already on this host; recreating the container is enough.`
      : `Pull ${target} and recreate the container.`}
  />
{:else if tracking.pinnedTag}
  <span
    class="muted note"
    title="This container uses the tag {tracking.pinnedTag} directly, so redeploying it won't move it to a newer version."
    >Pinned to {tracking.pinnedTag}</span
  >
{:else if tracking.status === "upToDate"}
  {#if showCurrent}
    <span class="muted note">Runs {tracking.tag ?? "the current image"}</span>
  {/if}
{:else if tracking.status === "outdated" || tracking.status === "pulledNotRecreated"}
  <span class="muted note" title="Not counted because the container isn't running.">Older version</span>
{:else if tracking.status === "internalRegistry"}
  <span
    class="muted note"
    title="Pulled from the internal registry by its real name. DockVMap can't compare it with the registry's tag right now: the tag may be gone, or the image was pulled by digest."
    >Internal registry</span
  >
{:else if tracking.status === "unknownImage"}
  <span class="muted note" title="Pulled from the proxy as {tracking.imageName}, which isn't a virtual image (any more)."
    >Unknown virtual image</span
  >
{:else}
  <span
    class="muted note"
    title="DockVMap doesn't know this image's current digest yet, or the container's image has no digest."
    >Version unknown</span
  >
{/if}

<style>
  .note {
    font-size: 0.8125rem;
  }
</style>
