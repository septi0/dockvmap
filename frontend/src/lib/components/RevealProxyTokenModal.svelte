<script lang="ts">
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert'
  import Copy from '@lucide/svelte/icons/copy'
  import Modal from './Modal.svelte'
  import Button from './Button.svelte'
  import { toast } from '../services/toast'
  import type { CreateProxyTokenResult } from '../api/types/proxyTokens'

  let {
    result,
    onClose,
  }: {
    result: CreateProxyTokenResult | null
    onClose: () => void
  } = $props()

  let copyError = $state<string | null>(null)

  async function copyToken() {
    if (!result) return

    copyError = null

    try {
      await navigator.clipboard.writeText(result.token)
      toast.success('Token copied to clipboard.')
    } catch {
      copyError = 'Could not copy automatically - select and copy the token manually.'
    }
  }
</script>

<Modal open={result !== null} {onClose} title="Token created" size="lg">
  {#if result}
    <p class="warning">
      <TriangleAlert size={16} strokeWidth={2} />
      Copy "{result.label}" now - it won't be shown again.
    </p>

    <div class="command-row">
      <code class="command">{result.token}</code>
      <button
        type="button"
        class="icon-button bordered"
        onclick={copyToken}
        aria-label="Copy token"
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

    <div class="actions">
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

  .actions {
    display: flex;
    justify-content: flex-end;
    margin-top: var(--space-4);
  }
</style>
