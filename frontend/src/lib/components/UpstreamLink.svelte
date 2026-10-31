<script lang="ts">
  import ExternalLink from "@lucide/svelte/icons/external-link";
  import { upstreamUrl } from "../utils/upstream";

  let { registry, repository }: { registry: string; repository: string } =
    $props();

  let href = $derived(upstreamUrl(registry, repository));
</script>

{#if href}
  <a class="upstream-link" {href} target="_blank" rel="noopener noreferrer">
    Upstream page
    <ExternalLink size={12} strokeWidth={2} />
  </a>
{/if}

<style>
  .upstream-link {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    font-size: 0.8125rem;
    color: var(--color-accent);
    text-decoration: none;
    white-space: nowrap;
  }

  .upstream-link:hover {
    text-decoration: underline;
  }
</style>
