<script lang="ts">
  import { link } from "svelte-spa-router";
  import Logo from "../lib/components/Logo.svelte";
  import PageTitle from "../lib/components/PageTitle.svelte";
</script>

<PageTitle title="Not Found" />

<div class="not-found">
  <Logo size="lg" />
  <h1>Page not found</h1>
  <a class="btn btn-secondary" href="/" use:link>Go home</a>
</div>

<style>
  .not-found {
    min-height: 100vh;
    min-height: 100dvh;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--space-4);
  }

  a {
    text-decoration: none;
  }
</style>
