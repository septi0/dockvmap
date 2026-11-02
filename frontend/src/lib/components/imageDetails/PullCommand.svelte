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

<div class="pull-command">
  <div class="command-row">
    <code class="command">{pullCommand}</code>
    <button
      type="button"
      class="icon-button bordered"
      onclick={copyPullCommand}
      aria-label="Copy pull command"
      title="Copy pull command"
    >
      <Copy size={16} strokeWidth={1.75} />
    </button>
  </div>

  {#if pullInfo && !pullInfo.hostConfigured}
    <p class="field-hint muted">
      Host based on the address used to reach this page. Set <code
        class="inline-code">proxy_public_host</code
      > in config if clients reach the registry through a different hostname.
    </p>
  {/if}

  {#if copyError}
    <p class="error">
      <TriangleAlert size={16} strokeWidth={2} />
      {copyError}
    </p>
  {/if}
</div>

<style>
  .pull-command {
    margin-bottom: var(--space-6);
  }

  .field-hint {
    margin: var(--space-2) 0 0;
    font-size: 0.8125rem;
  }
</style>
