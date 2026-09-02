<script lang="ts">
  import { push, link } from "svelte-spa-router";
  import Boxes from "@lucide/svelte/icons/boxes";
  import CircleArrowUp from "@lucide/svelte/icons/circle-arrow-up";
  import CircleCheck from "@lucide/svelte/icons/circle-check";
  import Plus from "@lucide/svelte/icons/plus";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Button from "../Button.svelte";
  import DashboardCard from "./DashboardCard.svelte";
  import CardBody from "./CardBody.svelte";
  import CardState from "./CardState.svelte";
  import { listImages } from "../../api/images";
  import { ApiError } from "../../api/client";
  import { dashboardRefresh } from "../../services/dashboardRefresh";
  import { dashboardSummary } from "../../services/dashboardSummary";
  import { formatNumber } from "../../utils/format";
  import type { Image } from "../../api/types/images";

  const CARD_ID = "updates-available";
  const LIMIT = 5;

  let images = $state<Image[]>([]);
  let total = $state(0);
  let loading = $state(true);
  let loaded = $state(false);
  let error = $state<string | null>(null);

  let refreshNonce = $derived($dashboardRefresh.nonce);

  $effect(() => {
    refreshNonce;
    void load();
  });

  async function load() {
    dashboardRefresh.begin(CARD_ID);
    loading = true;
    error = null;

    try {
      const [result] = await Promise.all([
        listImages({
          offset: 0,
          limit: LIMIT,
          status: "updateAvailable",
        }),
        dashboardSummary.load(),
      ]);

      images = result.images;
      total = result.total;
    } catch (err) {
      error = err instanceof ApiError ? err.message : "Failed to load updates";
    } finally {
      loading = false;
      loaded = true;
      dashboardRefresh.end(CARD_ID, !error);
    }
  }

  let trackedImages = $derived($dashboardSummary.data?.images.total ?? null);
</script>

<DashboardCard title="Updates available">
  {#snippet icon()}<CircleArrowUp size={16} strokeWidth={1.75} />{/snippet}

  <CardBody
    loading={loading && !loaded}
    busy={loading && loaded}
    hasError={!!error}
    empty={images.length === 0}
  >
    {#snippet errorState()}
      <CardState
        tone="error"
        title="Couldn’t load updates"
        description={error ?? undefined}
      >
        {#snippet icon()}<TriangleAlert size={30} strokeWidth={1.5} />{/snippet}
        {#snippet action()}
          <button
            type="button"
            class="link"
            onclick={() => dashboardRefresh.requestRefresh()}
          >
            Try again
          </button>
        {/snippet}
      </CardState>
    {/snippet}

    {#snippet emptyState()}
      {#if trackedImages === 0}
        <CardState
          title="No images tracked yet"
          description="Add a virtual image to start tracking upstream tags."
        >
          {#snippet icon()}<Boxes size={30} strokeWidth={1.5} />{/snippet}
          {#snippet action()}
            <Button onclick={() => push("/images/new")}>
              <Plus size={16} strokeWidth={2} />
              Add virtual image
            </Button>
          {/snippet}
        </CardState>
      {:else if trackedImages === null}
        <CardState title="No updates available">
          {#snippet icon()}<CircleArrowUp size={30} strokeWidth={1.5} />{/snippet}
        </CardState>
      {:else}
        <CardState
          tone="success"
          title="Everything is up to date"
          description="Every tracked image is on its latest matching tag."
        >
          {#snippet icon()}<CircleCheck size={30} strokeWidth={1.5} />{/snippet}
        </CardState>
      {/if}
    {/snippet}

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
            </span>
            {#if image.updateAvailableTag}
              <span class="item-delta">
                <span class="delta-from">{image.tag}</span>
                <span class="delta-arrow" aria-hidden="true">&rarr;</span>
                <span class="delta-to">{image.updateAvailableTag}</span>
              </span>
            {/if}
          </button>
        </li>
      {/each}
    </ul>

    {#if total > images.length}
      <a class="view-all link" href="/images?status=updateAvailable" use:link>
        View all {formatNumber(total)} images with updates available
      </a>
    {/if}
  </CardBody>
</DashboardCard>

<style>
  .item-delta {
    display: flex;
    align-items: center;
    flex-shrink: 0;
    gap: var(--space-1);
    font-size: 0.8125rem;
    white-space: nowrap;
  }

  .delta-from {
    color: var(--color-text-muted);
  }

  .delta-arrow {
    color: var(--color-text-faint);
  }

  .delta-to {
    font-weight: 500;
    color: var(--color-accent);
  }

  .view-all {
    display: inline-block;
    margin-top: var(--space-3);
  }
</style>
