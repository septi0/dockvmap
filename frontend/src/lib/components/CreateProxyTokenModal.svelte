<script lang="ts">
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert'
  import Modal from './Modal.svelte'
  import Field from './Field.svelte'
  import Button from './Button.svelte'
  import { createProxyToken } from '../api/proxyTokens'
  import { ApiError } from '../api/client'
  import type { CreateProxyTokenResult } from '../api/types/proxyTokens'

  let {
    open,
    onClose,
    onCreated,
  }: {
    open: boolean
    onClose: () => void
    onCreated: (result: CreateProxyTokenResult) => void
  } = $props()

  let label = $state('')
  let error = $state<string | null>(null)
  let submitting = $state(false)

  function reset() {
    label = ''
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
      const result = await createProxyToken({ label: trimmedLabel })
      onCreated(result)
      reset()
      onClose()
    } catch (err) {
      error = err instanceof ApiError ? err.message : 'Failed to create token'
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

    <div class="actions">
      <Button type="button" variant="secondary" onclick={handleClose}>Cancel</Button>
      <Button type="submit" disabled={submitting}>
        {submitting ? 'Creating…' : 'Create token'}
      </Button>
    </div>
  </form>
</Modal>

<style>
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }
</style>
