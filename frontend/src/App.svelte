<script lang="ts">
  import { onMount } from "svelte";
  import Router from "svelte-spa-router";
  import LoaderCircle from "@lucide/svelte/icons/loader-circle";
  import { auth } from "./lib/services/auth";
  import { theme } from "./lib/services/theme";
  import ToastContainer from "./lib/components/ToastContainer.svelte";
  import routes, { handleConditionsFailed } from "./routes";

  onMount(() => {
    auth.init();
    theme.init();
  });
</script>

{#if $auth.status === "loading"}
  <div class="app-loading">
    <span class="spin"><LoaderCircle size={28} strokeWidth={2.5} /></span>
  </div>
{:else}
  <Router {routes} onConditionsFailed={handleConditionsFailed} />
{/if}

<ToastContainer />

<style>
  .app-loading {
    position: fixed;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--color-accent);
  }
</style>
