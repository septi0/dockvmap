<script lang="ts">
  import CopyButton from "../CopyButton.svelte";
  import { pullCommand as buildPullCommand } from "../../utils/pullInfo";
  import type { PullInfo } from "../../api/types/metrics";

  let {
    pullInfo,
    imageName,
  }: { pullInfo: PullInfo | null; imageName: string } = $props();

  const pullCommand = $derived(
    pullInfo ? buildPullCommand(pullInfo, imageName) : "",
  );
</script>

<div class="pull-command">
  <div class="command-row">
    <code class="command">{pullCommand}</code>
    <CopyButton
      text={pullCommand}
      label="Copy pull command"
      copiedMessage="Pull command copied to clipboard."
      bordered
    />
  </div>

  {#if pullInfo && !pullInfo.hostConfigured}
    <p class="field-hint muted">
      Host based on the address used to reach this page. Set <code
        class="inline-code">proxy_public_host</code
      > in config if clients reach the registry through a different hostname.
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
