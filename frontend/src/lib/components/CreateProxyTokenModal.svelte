<script lang="ts">
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert'
  import Modal from './Modal.svelte'
  import Field from './Field.svelte'
  import Button from './Button.svelte'
  import { createProxyToken } from '../api/proxyTokens'
  import { errorMessage } from '../api/client'
  import type { CreateProxyTokenResult } from '../api/types/proxyTokens'

  let {
    open,
    internalRegistry,
    onClose,
    onCreated,
  }: {
    open: boolean
    internalRegistry: boolean
    onClose: () => void
    onCreated: (result: CreateProxyTokenResult) => void
  } = $props()

  let label = $state('')
  let canPush = $state(false)
  let error = $state<string | null>(null)
  let submitting = $state(false)

  function reset() {
    label = ''
    canPush = false
    error = null
    submitting = false
  }

  function handleClose() {
    reset()
    onClose()
  }

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault()
    error = null

    const trimmedLabel = label.trim()

    if (!trimmedLabel) {
      error = 'Label is required'
      return
    }

    submitting = true

    try {
      const result = await createProxyToken({ label: trimmedLabel, canPush })
      onCreated(result)
      reset()
      onClose()
    } catch (err) {
      error = errorMessage(err, 'Failed to create token')
    } finally {
      submitting = false
    }
  }
</script>

<Modal {open} onClose={handleClose} title="Create proxy token">
  <form onsubmit={handleSubmit}>
    {#if error}
      <p class="error">
        <TriangleAlert size={16} strokeWidth={2} />
        {error}
      </p>
    {/if}

    <Field
      label="Label"
      bind:value={label}
      placeholder="e.g. CI pipeline, homelab NAS"
      required
    />

    {#if internalRegistry}
      <div>
        <label class="checkbox">
          <input type="checkbox" bind:checked={canPush} />
          <span>Can push</span>
        </label>
        <p class="section-hint muted can-push-hint">
          For build scripts that push to the internal registry. Hosts only need to pull.
        </p>
      </div>
    {/if}

    <div class="modal-actions">
      <Button type="button" variant="secondary" onclick={handleClose}>Cancel</Button>
      <Button type="submit" disabled={submitting}>
        {submitting ? 'Creating…' : 'Create token'}
      </Button>
    </div>
  </form>
</Modal>

<style>
  .can-push-hint {
    margin: var(--space-1) 0 0;
    font-size: 0.8125rem;
  }
</style>
