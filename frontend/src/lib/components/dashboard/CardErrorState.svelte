<script lang="ts">
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import CardState from "./CardState.svelte";

  let {
    title,
    error,
    onRetry,
  }: {
    title: string;
    error?: string | null;
    onRetry: () => void;
  } = $props();
</script>

<CardState tone="error" {title} description={error ?? undefined}>
  {#snippet icon()}<TriangleAlert size={30} strokeWidth={1.5} />{/snippet}
  {#snippet action()}
    <button type="button" class="link" onclick={onRetry}>Try again</button>
  {/snippet}
</CardState>
