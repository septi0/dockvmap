<script lang="ts">
  import { push } from "svelte-spa-router";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import { auth } from "../lib/services/auth";
  import { ApiError } from "../lib/api/client";
  import { takeIntendedLocation } from "../routes";
  import AuthCard from "../lib/components/AuthCard.svelte";
  import PageTitle from "../lib/components/PageTitle.svelte";
  import Field from "../lib/components/Field.svelte";
  import Button from "../lib/components/Button.svelte";

  let username = $state("");
  let password = $state("");
  let error = $state<string | null>(null);
  let submitting = $state(false);

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    error = null;
    submitting = true;

    try {
      await auth.login(username.trim(), password);
      push(takeIntendedLocation());
    } catch (err) {
      error = err instanceof ApiError ? err.message : "Failed to sign in";
    } finally {
      submitting = false;
    }
  }
</script>

<PageTitle title="Sign In" />

<AuthCard title="Sign in">
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
      autofocus
    />
    <Field
      label="Password"
      type="password"
      bind:value={password}
      autocomplete="current-password"
      required
    />

    <Button type="submit" disabled={submitting}>
      {submitting ? "Signing in…" : "Sign in"}
    </Button>
  </form>
</AuthCard>
