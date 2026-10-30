<script lang="ts">
  import { onMount } from "svelte";
  import CircleArrowUp from "@lucide/svelte/icons/circle-arrow-up";
  import AppShell from "../lib/components/AppShell.svelte";
  import PageTitle from "../lib/components/PageTitle.svelte";
  import AsyncState from "../lib/components/AsyncState.svelte";
  import Pagination from "../lib/components/Pagination.svelte";
  import FilterBar from "../lib/components/FilterBar.svelte";
  import SearchInput from "../lib/components/SearchInput.svelte";
  import RefreshingIndicator from "../lib/components/RefreshingIndicator.svelte";
  import UpdateReviewRow from "../lib/components/UpdateReviewRow.svelte";
  import { listImages, IMAGES_PAGE_SIZE } from "../lib/api/images";
  import { tagRefreshStatus } from "../lib/services/tagRefreshStatus";
  import type { Image } from "../lib/api/types/images";
  import { formatNumber } from "../lib/utils/format";
  import { writeListQuery } from "../lib/utils/listQuery";
  import { createListView } from "../lib/utils/listView.svelte";

  const view = createListView<{ search: string; offset: number }, Image>({
    routePath: "/updates",
    defaults: { search: "", offset: 0 },
    errorFallback: "Failed to load updates",
    fetch: (q) =>
      listImages({
        offset: q.offset,
        limit: IMAGES_PAGE_SIZE,
        search: q.search || undefined,
        status: "updateAvailable",
      }).then((r) => ({ items: r.images, total: r.total })),
  });

  let reloadToken = $state(0);

  let sweepRunning = $derived($tagRefreshStatus.data?.running ?? false);
  let cleared = $derived(
    view.items.filter((image) => !image.updateAvailable).length,
  );
  let remaining = $derived(Math.max(0, view.total - cleared));
  let detailsQuery = $derived(
    writeListQuery({ from: "updates", ...view.filters }),
  );

  onMount(view.init);
  onMount(() => tagRefreshStatus.watch());
  onMount(() =>
    tagRefreshStatus.onCompleted(() => {
      reloadToken += 1;
      if (view.loaded && cleared === 0) view.reload();
    }),
  );

  function applyImage(fresh: Image) {
    const image = view.items.find((item) => item.id === fresh.id);
    if (image) Object.assign(image, fresh);
  }

  function changePage(next: number) {
    const target = next > view.filters.offset ? next - cleared : next;
    view.setOffset(Math.floor(target / IMAGES_PAGE_SIZE) * IMAGES_PAGE_SIZE);
  }
</script>

<PageTitle title="Updates" />

<AppShell>
  <div class="list-header">
    <div>
      <div class="title-row">
        <CircleArrowUp size={20} strokeWidth={1.75} />
        <h1>Updates</h1>
      </div>
      {#if view.loaded && !view.error}
        <p class="subtitle">
          {formatNumber(remaining)}
          {remaining === 1 ? "image" : "images"} with an update available
        </p>
      {/if}
    </div>
    {#if sweepRunning}
      <RefreshingIndicator text="Checking all images for tag changes…" />
    {/if}
  </div>

  <FilterBar active={view.hasActiveFilters} onClear={view.clear}>
    <SearchInput
      bind:value={view.filters.search}
      placeholder="Search images…"
      onSearch={view.applyFilters}
    />
  </FilterBar>

  <AsyncState
    loading={view.loading && !view.loaded}
    busy={view.loading && view.loaded}
    error={view.error}
    empty={view.items.length === 0}
    emptyMessage={view.filters.search
      ? "No images with an update match your search."
      : "All caught up. No images have an update available."}
    listSkeleton
  >
    {#each view.items as image (image.id)}
      <UpdateReviewRow
        {image}
        {detailsQuery}
        {sweepRunning}
        {reloadToken}
        onImageChanged={applyImage}
      />
    {/each}

    <Pagination
      total={view.total}
      limit={IMAGES_PAGE_SIZE}
      offset={view.filters.offset}
      onOffsetChange={changePage}
    />
  </AsyncState>
</AppShell>
