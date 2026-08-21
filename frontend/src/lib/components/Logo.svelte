<script lang="ts">
  import { link } from "svelte-spa-router";

  let { size = "md" }: { size?: "sm" | "md" | "lg" } = $props();

  const widths = { sm: 186, md: 220, lg: 260 };
</script>

<a class="logo" href="/" use:link>
  <img
    class="light"
    src="/images/logo.png"
    alt="DockVMap"
    style:width="{widths[size]}px"
  />
  <img
    class="dark"
    src="/images/logo-dark.png"
    alt="DockVMap"
    style:width="{widths[size]}px"
  />
</a>

<style>
  .logo {
    display: inline-flex;
  }

  img {
    display: block;
    height: auto;
  }

  .dark {
    display: none;
  }

  @media (prefers-color-scheme: dark) {
    :global(:root:not([data-theme="light"])) .light {
      display: none;
    }

    :global(:root:not([data-theme="light"])) .dark {
      display: block;
    }
  }

  :global(:root[data-theme="dark"]) .light {
    display: none;
  }

  :global(:root[data-theme="dark"]) .dark {
    display: block;
  }
</style>
