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
  import {
    IMAGE_GROUPS_MAX_LIMIT,
    listImageGroups,
  } from "../lib/api/imageGroups";
  import { tagRefreshStatus } from "../lib/services/tagRefreshStatus";
  import type { Image } from "../lib/api/types/images";
  import type { ImageGroup } from "../lib/api/types/imageGroups";
  import { formatNumber } from "../lib/utils/format";
  import { writeListQuery } from "../lib/utils/listQuery";
  import { createListView } from "../lib/utils/listView.svelte";

  type UpdateItem = { key: string; image: Image; group: ImageGroup | null };

  const view = createListView<{ search: string; offset: number }, UpdateItem>({
    routePath: "/updates",
    defaults: { search: "", offset: 0 },
    errorFallback: "Failed to load updates",
    fetch: async (q) => {
      const search = q.search || undefined;
      const [images, groups] = await Promise.all([
        listImages({
          offset: q.offset,
          limit: IMAGES_PAGE_SIZE,
          search,
          status: "updateAvailable",
          ungrouped: true,
        }),
        listImageGroups({
          limit: IMAGE_GROUPS_MAX_LIMIT,
          search,
          status: "updateAvailable",
        }),
      ]);

      return {
        items: [
          ...groups.groups.map((group) => ({
            key: `group-${group.id}`,
            image: group.images[0],
            group,
          })),
          ...images.images.map((image) => ({
            key: `image-${image.id}`,
            image,
            group: null,
          })),
        ],
        total: images.total + groups.groups.length,
      };
    },
  });

  let reloadToken = $state(0);

  let sweepRunning = $derived($tagRefreshStatus.data?.running ?? false);
  let groupItems = $derived(view.items.filter((item) => item.group));
  let imageItems = $derived(view.items.filter((item) => !item.group));
  let visibleItems = $derived(
    view.filters.offset === 0 ? view.items : imageItems,
  );
  let clearedGroups = $derived(
    groupItems.filter((item) => !item.group?.updateAvailable).length,
  );
  let cleared = $derived(
    imageItems.filter((item) => !item.image.updateAvailable).length,
  );
  let remainingGroups = $derived(groupItems.length - clearedGroups);
  let remainingImages = $derived(
    Math.max(0, view.total - groupItems.length - cleared),
  );
  let summary = $derived(
    remainingGroups > 0
      ? `${formatNumber(remainingGroups)} ${remainingGroups === 1 ? "group" : "groups"} and ${formatNumber(remainingImages)} ${remainingImages === 1 ? "image" : "images"} with an update available`
      : `${formatNumber(remainingImages)} ${remainingImages === 1 ? "image" : "images"} with an update available`,
  );
  let detailsQuery = $derived(
    writeListQuery({ from: "updates", ...view.filters }),
  );

  onMount(view.init);
  onMount(() => tagRefreshStatus.watch());
  onMount(() =>
    tagRefreshStatus.onCompleted(() => {
      reloadToken += 1;
      if (view.loaded && cleared + clearedGroups === 0) view.reload();
    }),
  );

  function applyImage(fresh: Image) {
    const item = view.items.find(
      (candidate) => !candidate.group && candidate.image.id === fresh.id,
    );
    if (item) Object.assign(item.image, fresh);
  }

  function applyGroup(fresh: ImageGroup) {
    const item = view.items.find((candidate) => candidate.group?.id === fresh.id);

    if (item) {
      item.group = fresh;
      item.image = fresh.images[0];
    }
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
        <p class="subtitle">{summary}</p>
      {/if}
    </div>
    {#if sweepRunning}
      <RefreshingIndicator text="Checking all images for tag changes…" />
    {/if}
  </div>

  <FilterBar active={view.hasActiveFilters} onClear={view.clear}>
    <SearchInput
      bind:value={view.filters.search}
      placeholder="Search images and groups…"
      onSearch={view.applyFilters}
    />
  </FilterBar>

  <AsyncState
    loading={view.loading && !view.loaded}
    busy={view.loading && view.loaded}
    error={view.error}
    empty={visibleItems.length === 0}
    emptyMessage={view.filters.search
      ? "No images with an update match your search."
      : "No newer tags. Every image is on its latest matching tag."}
    listSkeleton
  >
    {#each visibleItems as item (item.key)}
      <UpdateReviewRow
        image={item.image}
        group={item.group}
        {detailsQuery}
        {sweepRunning}
        {reloadToken}
        onImageChanged={applyImage}
        onGroupChanged={applyGroup}
        onStructureChanged={view.reload}
      />
    {/each}

    <Pagination
      total={view.total - groupItems.length}
      limit={IMAGES_PAGE_SIZE}
      offset={view.filters.offset}
      onOffsetChange={changePage}
    />
  </AsyncState>
</AppShell>
