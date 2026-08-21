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
        class="icon-button"
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

  .command-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .command {
    flex: 1;
    min-width: 0;
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    background: var(--color-bg);
    border: 1px solid var(--color-border);
    font-family: ui-monospace, monospace;
    font-size: 0.8125rem;
    overflow-x: auto;
    white-space: pre;
  }

  .icon-button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-2);
    border: none;
    background: transparent;
    border-radius: var(--radius-sm);
    color: var(--color-text-muted);
    cursor: pointer;
    flex-shrink: 0;
  }

  .icon-button:hover {
    background: var(--color-surface-hover);
    color: var(--color-text);
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    margin-top: var(--space-4);
  }
</style>
