<script lang="ts">
  import { push } from "svelte-spa-router";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import { auth } from "../lib/services/auth";
  import { errorMessage } from "../lib/api/client";
  import AuthCard from "../lib/components/AuthCard.svelte";
  import PageTitle from "../lib/components/PageTitle.svelte";
  import Field from "../lib/components/Field.svelte";
  import Button from "../lib/components/Button.svelte";

  let username = $state("");
  let email = $state("");
  let password = $state("");
  let confirmPassword = $state("");
  let error = $state<string | null>(null);
  let submitting = $state(false);

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    error = null;

    if (password !== confirmPassword) {
      error = "Passwords do not match";
      return;
    }

    submitting = true;

    try {
      await auth.bootstrap(username.trim(), email.trim(), password);
      push("/");
    } catch (err) {
      error = errorMessage(err, "Failed to create account");
    } finally {
      submitting = false;
    }
  }
</script>

<PageTitle title="Setup" />

<AuthCard title="Create your account">
  <p class="hint">
    Set up the account you'll use to sign in to DockVMap. This step is only
    available once.
  </p>

  <form onsubmit={handleSubmit}>
    {#if error}
      <p class="error">
        <TriangleAlert size={16} strokeWidth={2} />
        {error}
      </p>
    {/if}

    <Field
      label="Username"
      bind:value={username}
      autocomplete="username"
      required
    />
    <Field
      label="Email"
      type="email"
      bind:value={email}
      autocomplete="email"
      required
    />
    <Field
      label="Password"
      type="password"
      bind:value={password}
      autocomplete="new-password"
      required
    />
    <Field
      label="Confirm password"
      type="password"
      bind:value={confirmPassword}
      autocomplete="new-password"
      required
    />

    <Button type="submit" disabled={submitting}>
      {submitting ? "Creating…" : "Create account"}
    </Button>
  </form>
</AuthCard>

<style>
  .hint {
    font-size: 0.875rem;
    color: var(--color-text-muted);
  }
</style>
