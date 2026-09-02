<script lang="ts">
  import { onMount } from 'svelte'
  import Plus from '@lucide/svelte/icons/plus'
  import KeyRound from '@lucide/svelte/icons/key-round'
  import Trash2 from '@lucide/svelte/icons/trash-2'
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert'
  import AppShell from '../lib/components/AppShell.svelte'
  import PageTitle from '../lib/components/PageTitle.svelte'
  import AsyncState from '../lib/components/AsyncState.svelte'
  import Button from '../lib/components/Button.svelte'
  import CreateProxyTokenModal from '../lib/components/CreateProxyTokenModal.svelte'
  import RevealProxyTokenModal from '../lib/components/RevealProxyTokenModal.svelte'
  import ConfirmDialog from '../lib/components/ConfirmDialog.svelte'
  import { listProxyTokens, deleteProxyToken } from '../lib/api/proxyTokens'
  import { getProxyAuthStatus } from '../lib/api/system'
  import { errorMessage } from '../lib/api/client'
  import { toast } from '../lib/services/toast'
  import { formatDate } from '../lib/utils/format'
  import type { ProxyToken, CreateProxyTokenResult } from '../lib/api/types/proxyTokens'

  let tokens = $state<ProxyToken[]>([])
  let loading = $state(true)
  let loaded = $state(false)
  let error = $state<string | null>(null)
  let showCreateModal = $state(false)
  let revealedToken = $state<CreateProxyTokenResult | null>(null)
  let deletingToken = $state<ProxyToken | null>(null)
  let deleteError = $state<string | null>(null)
  let deleting = $state(false)
  let proxyAuthEnabled = $state(true)
  let loadToken = 0

  async function load() {
    const requestId = ++loadToken
    loading = true
    error = null

    try {
      const result = await listProxyTokens()
      if (requestId !== loadToken) return
      tokens = result
    } catch (err) {
      if (requestId !== loadToken) return
      error = errorMessage(err, 'Failed to load proxy tokens')
    } finally {
      if (requestId === loadToken) {
        loading = false
        loaded = true
      }
    }
  }

  onMount(load)

  onMount(async () => {
    try {
      const status = await getProxyAuthStatus()
      proxyAuthEnabled = status.enabled
    } catch {}
  })

  async function handleCreated(result: CreateProxyTokenResult) {
    revealedToken = result
    await load()
  }

  function cancelDelete() {
    deletingToken = null
    deleteError = null
  }

  async function handleDelete() {
    if (!deletingToken) return

    deleteError = null
    deleting = true

    try {
      const deletedLabel = deletingToken.label
      await deleteProxyToken(deletingToken.id)
      deletingToken = null
      await load()
      toast.success(`Token "${deletedLabel}" deleted.`)
    } catch (err) {
      deleteError = errorMessage(err, 'Failed to delete token')
    } finally {
      deleting = false
    }
  }
</script>

<PageTitle title='Proxy Tokens' />

<AppShell>
  <div class="list-header">
    <div class="title-row">
      <KeyRound size={20} strokeWidth={1.75} />
      <h1>Proxy Tokens</h1>
    </div>
    <Button onclick={() => (showCreateModal = true)}>
      <Plus size={16} strokeWidth={2} />
      Create token
    </Button>
  </div>

  {#if !proxyAuthEnabled}
    <p class="warning-text">
      <TriangleAlert size={14} strokeWidth={2} />
      <span>
        Proxy authentication is <strong>not enabled</strong> in config.yaml. These
        tokens exist but nothing is being enforced yet; the proxy accepts pulls
        with no credential until this is turned on.
      </span>
    </p>
  {/if}

  <AsyncState
    loading={loading && !loaded}
    busy={loading && loaded}
    {error}
    empty={tokens.length === 0}
    emptyMessage="No proxy tokens yet. Create one to authenticate a client against the proxy."
    columns={3}
  >
    {#snippet emptyAction()}
      <Button onclick={() => (showCreateModal = true)}>
        <Plus size={16} strokeWidth={2} />
        Create token
      </Button>
    {/snippet}

    <div class="card table-wrap">
      <table class="table">
        <thead>
          <tr>
            <th>Label</th>
            <th>Created</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each tokens as token (token.id)}
            <tr>
              <td>{token.label}</td>
              <td>{formatDate(token.createdAt)}</td>
              <td class="actions">
                <button
                  type="button"
                  class="icon-button danger"
                  onclick={() => (deletingToken = token)}
                  aria-label="Delete {token.label}"
                >
                  <Trash2 size={16} strokeWidth={1.75} />
                </button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </AsyncState>
</AppShell>

<CreateProxyTokenModal
  open={showCreateModal}
  onClose={() => (showCreateModal = false)}
  onCreated={handleCreated}
/>

<RevealProxyTokenModal result={revealedToken} onClose={() => (revealedToken = null)} />

<ConfirmDialog
  open={deletingToken !== null}
  title="Delete proxy token"
  message={`Delete "${deletingToken?.label ?? ''}"? Any client using it will stop being able to authenticate. This cannot be undone.`}
  confirmLabel="Delete"
  danger
  error={deleteError}
  submitting={deleting}
  onConfirm={handleDelete}
  onCancel={cancelDelete}
/>

<style>
  .warning-text {
    margin-bottom: var(--space-4);
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-1);
  }
</style>
