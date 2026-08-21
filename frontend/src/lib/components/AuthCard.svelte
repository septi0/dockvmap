<script lang="ts">
  import type { Snippet } from "svelte";
  import Logo from "./Logo.svelte";

  let { title, children }: { title: string; children: Snippet } = $props();
</script>

<div class="auth-page">
  <div class="auth-card card">
    <div class="logo-wrap"><Logo size="lg" /></div>
    <h1>{title}</h1>
    {@render children()}
  </div>
</div>

<style>
  .auth-page {
    min-height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-4);
  }

  .auth-card {
    width: 100%;
    max-width: 360px;
    padding: var(--space-6);
    box-shadow: var(--shadow-md);
  }

  .logo-wrap {
    display: flex;
    justify-content: center;
    margin-bottom: var(--space-5);
  }

  h1 {
    font-size: 1.25rem;
    text-align: center;
  }
</style>
