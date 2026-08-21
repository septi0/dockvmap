<script lang="ts">
  import Button from "./Button.svelte";

  let {
    total,
    limit,
    offset,
    onOffsetChange,
  }: {
    total: number;
    limit: number;
    offset: number;
    onOffsetChange: (offset: number) => void;
  } = $props();

  let currentPage = $derived(Math.floor(offset / limit) + 1);
  let totalPages = $derived(Math.max(1, Math.ceil(total / limit)));
  let hasPrev = $derived(currentPage > 1);
  let hasNext = $derived(currentPage < totalPages);

  function goPrev() {
    onOffsetChange(Math.max(0, offset - limit));
  }

  function goNext() {
    onOffsetChange(offset + limit);
  }
</script>

<div class="pagination">
  <span class="muted">Page {currentPage} of {totalPages}</span>

  <div class="buttons">
    <Button variant="secondary" disabled={!hasPrev} onclick={goPrev}
      >Previous</Button
    >
    <Button variant="secondary" disabled={!hasNext} onclick={goNext}
      >Next</Button
    >
  </div>
</div>

<style>
  .pagination {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    margin-top: var(--space-4);
  }

  .buttons {
    display: flex;
    gap: var(--space-2);
  }
</style>
