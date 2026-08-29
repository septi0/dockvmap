<script lang="ts">
  import { onMount } from "svelte";
  import { push, link } from "svelte-spa-router";
  import CircleArrowUp from "@lucide/svelte/icons/circle-arrow-up";
  import ArrowUp from "@lucide/svelte/icons/arrow-up";
  import AsyncState from "../AsyncState.svelte";
  import { listImages } from "../../api/images";
  import { ApiError } from "../../api/client";
  import { formatNumber } from "../../utils/format";
  import type { Image } from "../../api/types/images";

  const LIMIT = 6;

  let images = $state<Image[]>([]);
  let total = $state(0);
  let loading = $state(true);
  let error = $state<string | null>(null);

  async function load() {
    loading = true;
    error = null;

    try {
      const result = await listImages({
        offset: 0,
        limit: LIMIT,
        status: "updateAvailable",
      });
      images = result.images;
      total = result.total;
    } catch (err) {
      error = err instanceof ApiError ? err.message : "Failed to load updates";
    } finally {
      loading = false;
    }
  }

  onMount(load);
</script>

<div class="card section-card">
  <div class="card-head">
    <div class="card-head-title">
      <CircleArrowUp size={16} strokeWidth={1.75} />
      <h2>Updates available</h2>
    </div>
  </div>

  <AsyncState
    {loading}
    {error}
    empty={images.length === 0}
    emptyMessage="Everything is up to date."
  >
    <ul class="item-list">
      {#each images as image (image.id)}
        <li>
          <button
            type="button"
            class="item-row"
            onclick={() => push(`/images/${image.id}`)}
          >
            <span class="item-main">
              <span class="item-title">{image.name}</span>
              <span class="item-sub muted">
                {image.registry}/{image.repository}
              </span>
              {#if image.updateAvailableTag}
                <span class="item-sub muted">
                  {image.tag} &rarr; {image.updateAvailableTag}
                </span>
              {/if}
            </span>
            <span class="badge badge-warning">
              <ArrowUp size={12} strokeWidth={2.5} />
              Update
            </span>
          </button>
        </li>
      {/each}
    </ul>

    {#if total > images.length}
      <a class="view-all link" href="/images?status=updateAvailable" use:link>
        View all {formatNumber(total)} images with updates available
      </a>
    {/if}
  </AsyncState>
</div>

<style>
  .view-all {
    display: inline-block;
    margin-top: var(--space-3);
  }
</style>
