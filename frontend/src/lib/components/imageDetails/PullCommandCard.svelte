<script lang="ts">
  import Copy from "@lucide/svelte/icons/copy";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import { copyText } from "../../services/clipboard";
  import { toast } from "../../services/toast";
  import { pullCommand as buildPullCommand } from "../../utils/pullInfo";
  import type { PullInfo } from "../../api/types/metrics";

  let {
    pullInfo,
    imageName,
  }: { pullInfo: PullInfo | null; imageName: string } = $props();

  let copyError = $state<string | null>(null);

  const pullCommand = $derived(
    pullInfo ? buildPullCommand(pullInfo, imageName) : "",
  );

  async function copyPullCommand() {
    copyError = null;

    if (await copyText(pullCommand)) {
      toast.success("Pull command copied to clipboard.");
    } else {
      copyError =
        "Could not copy automatically - select and copy the command manually.";
    }
  }
</script>

<div class="card section-card">
  <h2>Pull command</h2>
  <p class="section-hint muted">
    Clients pull this virtual image using a stable tag that always
    resolves to the version you choose above.
  </p>

  <p class="field-hint muted">
    {#if pullInfo?.hostConfigured}
      Host pinned via the <code class="inline-code"
        >proxy_public_host</code
      > config option.
    {:else}
      Host based on the address used to reach this page. Set <code
        class="inline-code">proxy_public_host</code
      > in config if clients reach the registry through a different hostname.
    {/if}
  </p>

  <div class="command-row">
    <code class="command">{pullCommand}</code>
    <button
      type="button"
      class="icon-button bordered"
      onclick={copyPullCommand}
      aria-label="Copy pull command"
    >
      <Copy size={16} strokeWidth={1.75} />
    </button>
  </div>

  {#if copyError}
    <p class="error">
      <TriangleAlert size={16} strokeWidth={2} />
      {copyError}
    </p>
  {/if}
</div>

<style>
  .field-hint {
    margin: var(--space-1) 0 var(--space-4);
    font-size: 0.8125rem;
  }
</style>
