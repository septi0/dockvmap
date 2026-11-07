<script lang="ts">
  import Copy from "@lucide/svelte/icons/copy";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import { copyText } from "../services/clipboard";
  import { toast } from "../services/toast";

  let {
    text,
    label,
    copiedMessage,
    buttonText,
    bordered = false,
  }: { text: string; label: string; copiedMessage: string; buttonText?: string; bordered?: boolean } = $props();

  let failed = $state(false);

  async function copy() {
    failed = !(await copyText(text));

    if (!failed) toast.success(copiedMessage);
  }
</script>

{#if buttonText}
  <button type="button" class="btn btn-secondary btn-sm" onclick={copy} title={failed ? "Could not copy" : label}>
    {#if failed}
      <TriangleAlert size={14} strokeWidth={2} />
    {:else}
      <Copy size={14} strokeWidth={2} />
    {/if}
    {buttonText}
  </button>
{:else}
  <button
    type="button"
    class="icon-button"
    class:bordered
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
{/if}
