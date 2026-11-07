<script lang="ts">
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert'
  import Modal from './Modal.svelte'
  import Button from './Button.svelte'
  import CopyButton from './CopyButton.svelte'
  import type { CreateProxyTokenResult } from '../api/types/proxyTokens'

  let {
    result,
    onClose,
  }: {
    result: CreateProxyTokenResult | null
    onClose: () => void
  } = $props()
</script>

<Modal open={result !== null} {onClose} title="Token created" size="lg">
  {#if result}
    <p class="warning">
      <TriangleAlert size={16} strokeWidth={2} />
      Copy "{result.label}" now - it won't be shown again.
    </p>

    <div class="command-row">
      <code class="command">{result.token}</code>
      <CopyButton text={result.token} label="Copy token" copiedMessage="Token copied to clipboard." bordered />
    </div>

    <div class="modal-actions spaced">
      <Button onclick={onClose}>Done</Button>
    </div>
  {/if}
</Modal>

<style>
  .warning {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin: 0 0 var(--space-4);
    color: var(--color-warning);
  }
</style>
