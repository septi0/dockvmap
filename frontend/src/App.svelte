<script lang="ts">
  import { onMount } from "svelte";
  import Router from "svelte-spa-router";
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
  <p>Loading…</p>
{:else}
  <Router {routes} onConditionsFailed={handleConditionsFailed} />
{/if}

<ToastContainer />
