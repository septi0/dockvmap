<script lang="ts">
  import Copy from "@lucide/svelte/icons/copy";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import { copyText } from "../services/clipboard";
  import { toast } from "../services/toast";

  let {
    text,
    label,
    copiedMessage,
  }: { text: string; label: string; copiedMessage: string } = $props();

  let failed = $state(false);

  async function copy() {
    failed = !(await copyText(text));

    if (!failed) toast.success(copiedMessage);
  }
</script>

<button
  type="button"
  class="icon-button"
  onclick={copy}
  aria-label={failed ? "Could not copy" : label}
  title={failed ? "Could not copy" : label}
>
  {#if failed}
    <TriangleAlert size={16} strokeWidth={1.75} />
  {:else}
    <Copy size={16} strokeWidth={1.75} />
  {/if}
</button>
