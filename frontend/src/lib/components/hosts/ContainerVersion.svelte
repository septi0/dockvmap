<script lang="ts">
  import type { ContainerTracking } from "../../api/types/hosts";

  let { tracking }: { tracking: ContainerTracking } = $props();
</script>

<span class="container-version">
  {#if tracking.status === "upToDate"}
    <span class="muted">Up to date</span>
  {:else if tracking.status === "outdated"}
    <span
      class={tracking.needsUpdate ? "badge badge-warning" : "muted"}
      title={tracking.internalRegistry
        ? "A newer image was pushed to this tag in the internal registry. Pull it and recreate the container."
        : "This container runs an older version than the one DockVMap points to."}
      >Outdated</span
    >
  {:else if tracking.status === "pulledNotRecreated"}
    <span
      class={tracking.needsUpdate ? "badge badge-warning" : "muted"}
      title="The current version is already on this host, but the container still runs the old one. Recreate the container to apply it."
      >Pulled, not recreated</span
    >
  {:else if tracking.status === "internalRegistry"}
    <span
      class="muted"
      title="Pulled from the internal registry by its real name. DockVMap can't compare it with the registry's tag right now: the tag may be gone, or the image was pulled by digest."
      >Internal registry</span
    >
  {:else if tracking.status === "unknownImage"}
    <span
      class="muted"
      title="Pulled from the proxy as {tracking.imageName}, which isn't a virtual image (any more)."
      >Unknown virtual image</span
    >
  {:else}
    <span
      class="muted"
      title="DockVMap doesn't know this image's current digest yet, or the container's image has no digest."
      >Version unknown</span
    >
  {/if}

  {#if tracking.pinnedTag}
    <span
      class="muted pinned"
      title="This container uses the tag {tracking.pinnedTag} directly, so recreating it won't move it to a newer version."
      >pinned to {tracking.pinnedTag}</span
    >
  {/if}
</span>

<style>
  .container-version {
    display: inline-flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1) var(--space-2);
  }

  .badge {
    white-space: nowrap;
  }

  .pinned {
    font-size: 0.8125rem;
  }
</style>
