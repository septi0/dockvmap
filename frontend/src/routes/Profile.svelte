<script lang="ts">
  import UserRound from "@lucide/svelte/icons/user-round";
  import Pencil from "@lucide/svelte/icons/pencil";
  import Save from "@lucide/svelte/icons/save";
  import KeyRound from "@lucide/svelte/icons/key-round";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import AppShell from "../lib/components/AppShell.svelte";
  import PageTitle from "../lib/components/PageTitle.svelte";
  import AsyncState from "../lib/components/AsyncState.svelte";
  import ConfirmDialog from "../lib/components/ConfirmDialog.svelte";
  import DetailRow from "../lib/components/DetailRow.svelte";
  import Field from "../lib/components/Field.svelte";
  import Button from "../lib/components/Button.svelte";
  import ChangePasswordModal from "../lib/components/ChangePasswordModal.svelte";
  import { onMount } from "svelte";
  import { updateEmail, updatePreferences } from "../lib/api/users";
  import { getSMTPStatus } from "../lib/api/system";
  import { listSessions, invalidateSession } from "../lib/api/sessions";
  import { ApiError } from "../lib/api/client";
  import { auth } from "../lib/services/auth";
  import { formatDate } from "../lib/utils/format";
  import type { Session } from "../lib/api/types/sessions";
  import type { NotifyLevel } from "../lib/api/types/auth";

  let initial = $derived($auth.user?.username?.[0]?.toUpperCase() ?? "?");

  let editingEmail = $state(false);
  let email = $state($auth.user?.email ?? "");
  let emailError = $state<string | null>(null);
  let emailSubmitting = $state(false);

  let notifyLevel = $state<NotifyLevel>($auth.user?.notifyLevel ?? "all");
  let notifyError = $state<string | null>(null);
  let notifySubmitting = $state(false);

  const notifyLevelOptions: { value: NotifyLevel; label: string }[] = [
    { value: "all", label: "Any new or removed tag" },
    { value: "upgrades", label: "Only when an upgrade is available" },
    { value: "none", label: "Never" },
  ];

  let showPasswordModal = $state(false);

  let smtpEnabled = $state(true);

  let sessions = $state<Session[]>([]);
  let sessionsLoading = $state(true);
  let sessionsError = $state<string | null>(null);

  let revokingSession = $state<Session | null>(null);
  let revokeError = $state<string | null>(null);
  let revokeSubmitting = $state(false);

  onMount(async () => {
    try {
      const status = await getSMTPStatus();
      smtpEnabled = status.enabled;
    } catch {}
  });

  onMount(loadSessions);

  async function loadSessions() {
    sessionsLoading = true;
    sessionsError = null;

    try {
      const result = await listSessions();
      sessions = result.sessions;
    } catch (err) {
      sessionsError =
        err instanceof ApiError ? err.message : "Failed to load sessions";
    } finally {
      sessionsLoading = false;
    }
  }

  function confirmRevoke(session: Session) {
    revokeError = null;
    revokingSession = session;
  }

  function cancelRevoke() {
    revokingSession = null;
    revokeError = null;
  }

  async function handleRevokeConfirm() {
    if (!revokingSession) return;

    revokeSubmitting = true;
    revokeError = null;

    try {
      await invalidateSession(revokingSession.id);
      revokingSession = null;
      await loadSessions();
    } catch (err) {
      revokeError =
        err instanceof ApiError ? err.message : "Failed to revoke session";
    } finally {
      revokeSubmitting = false;
    }
  }

  function startEditingEmail() {
    email = $auth.user?.email ?? "";
    emailError = null;
    editingEmail = true;
  }

  function cancelEditingEmail() {
    editingEmail = false;
    emailError = null;
  }

  async function handleEmailSubmit(event: SubmitEvent) {
    event.preventDefault();
    emailError = null;
    emailSubmitting = true;

    try {
      await updateEmail(email.trim());
      await auth.refresh();
      editingEmail = false;
    } catch (err) {
      emailError =
        err instanceof ApiError ? err.message : "Failed to update email";
    } finally {
      emailSubmitting = false;
    }
  }

  async function handleNotifyChange() {
    const previous = $auth.user?.notifyLevel ?? "all";
    const next = notifyLevel;
    notifyError = null;
    notifySubmitting = true;

    try {
      await updatePreferences({ notifyLevel: next });
      await auth.refresh();
    } catch (err) {
      notifyLevel = previous;
      notifyError =
        err instanceof ApiError
          ? err.message
          : "Failed to update notification preference";
    } finally {
      notifySubmitting = false;
    }
  }
</script>

<PageTitle title="Profile" />

<AppShell>
  <div class="page-header">
    <div class="title-row">
      <UserRound size={20} strokeWidth={1.75} />
      <h1>Profile</h1>
    </div>
    <p class="subtitle">
      Manage your account details and notification preferences.
    </p>
  </div>

  <div class="card section-card">
    <div class="profile-header">
      <span class="profile-avatar">{initial}</span>
      <div class="profile-identity">
        <h2 class="profile-username">{$auth.user?.username}</h2>
        <p class="profile-email">{$auth.user?.email}</p>
      </div>
      {#if !editingEmail}
        <div class="profile-actions">
          <Button variant="secondary" size="sm" onclick={startEditingEmail}>
            <Pencil size={14} strokeWidth={2} />
            Edit email
          </Button>
          <Button
            variant="secondary"
            size="sm"
            onclick={() => (showPasswordModal = true)}
          >
            <KeyRound size={14} strokeWidth={2} />
            Change password
          </Button>
        </div>
      {/if}
    </div>

    {#if editingEmail}
      <form class="inline-form" onsubmit={handleEmailSubmit}>
        {#if emailError}
          <p class="error">
            <TriangleAlert size={16} strokeWidth={2} />
            {emailError}
          </p>
        {/if}

        <Field
          label="Email"
          type="email"
          bind:value={email}
          autocomplete="email"
          required
        />

        <div class="inline-form-actions">
          <Button type="submit" size="sm" disabled={emailSubmitting}>
            <Save size={14} strokeWidth={2} />
            {emailSubmitting ? "Saving…" : "Save"}
          </Button>
          <Button
            type="button"
            variant="secondary"
            size="sm"
            disabled={emailSubmitting}
            onclick={cancelEditingEmail}
          >
            Cancel
          </Button>
        </div>
      </form>
    {/if}
  </div>

  <div class="card section-card">
    <h2>Preferences</h2>

    <DetailRow label="Tag alert emails">
      <div class="detail-cell">
        {#if notifyError}
          <p class="error">
            <TriangleAlert size={16} strokeWidth={2} />
            {notifyError}
          </p>
        {/if}

        <label class="field">
          <span class="field-label"
            >Email me about tag changes for my virtual images</span
          >
          <select
            class="input"
            bind:value={notifyLevel}
            disabled={notifySubmitting}
            onchange={handleNotifyChange}
          >
            {#each notifyLevelOptions as option (option.value)}
              <option value={option.value}>{option.label}</option>
            {/each}
          </select>
        </label>

        {#if notifyLevel !== "none" && !smtpEnabled}
          <p class="warning-text">
            <TriangleAlert size={14} strokeWidth={2} />
            SMTP is not configured, so tag alert emails won't be sent until
            an administrator sets it up.
          </p>
        {/if}
      </div>
    </DetailRow>
  </div>

  <div class="card section-card">
    <h2>Active sessions</h2>
    <p class="section-hint muted">
      Devices currently signed in to your account. You can revoke any session
      except the one you're using right now.
    </p>

    <AsyncState
      loading={sessionsLoading}
      error={sessionsError}
      empty={sessions.length === 0}
      emptyMessage="No active sessions."
    >
      <ul class="item-list">
        {#each sessions as session (session.id)}
          <li>
            <div class="session-row">
              <div class="item-main">
                <span class="item-title"
                  >{session.ip || "Unknown IP"} · {session.userAgent ||
                    "Unknown device"}</span
                >
                <span class="item-sub muted">
                  Signed in {formatDate(session.createdAt)} · expires {formatDate(
                    session.expiresAt,
                  )}
                </span>
              </div>
              {#if session.current}
                <span class="badge badge-accent">Current</span>
              {:else}
                <Button
                  variant="danger"
                  size="sm"
                  onclick={() => confirmRevoke(session)}
                >
                  Revoke
                </Button>
              {/if}
            </div>
          </li>
        {/each}
      </ul>
    </AsyncState>
  </div>
</AppShell>

<ChangePasswordModal
  open={showPasswordModal}
  onClose={() => (showPasswordModal = false)}
/>

<ConfirmDialog
  open={revokingSession !== null}
  title="Revoke session"
  message="Sign out this session? Whoever is using it will need to log in again."
  confirmLabel="Revoke"
  danger
  error={revokeError}
  submitting={revokeSubmitting}
  onConfirm={handleRevokeConfirm}
  onCancel={cancelRevoke}
/>

<style>
  .profile-header {
    display: flex;
    align-items: center;
    gap: var(--space-4);
  }

  .profile-avatar {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    width: 56px;
    height: 56px;
    border-radius: var(--radius-full);
    background: var(--color-accent-muted-bg);
    color: var(--color-accent);
    font-size: 1.25rem;
    font-weight: 600;
  }

  .profile-identity {
    flex: 1;
    min-width: 0;
  }

  .profile-username {
    margin: 0;
    font-size: 1.125rem;
  }

  .profile-email {
    margin: var(--space-1) 0 0;
    color: var(--color-text-muted);
    font-size: 0.875rem;
  }

  .profile-actions {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    flex-shrink: 0;
  }

  .inline-form {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    max-width: 360px;
    margin-top: var(--space-4);
  }

  .inline-form-actions {
    display: flex;
    gap: var(--space-2);
  }

  .session-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    padding: var(--space-2) 0;
    border-bottom: 1px solid var(--color-border);
  }

  li:last-child .session-row {
    border-bottom: none;
  }
</style>
