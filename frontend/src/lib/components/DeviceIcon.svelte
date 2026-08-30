<script lang="ts">
  import Monitor from "@lucide/svelte/icons/monitor";
  import Smartphone from "@lucide/svelte/icons/smartphone";
  import Tablet from "@lucide/svelte/icons/tablet";
  import CircleHelp from "@lucide/svelte/icons/circle-help";
  import type { DeviceType } from "../utils/userAgent";

  let { device, size = 14 }: { device: DeviceType; size?: number } = $props();
</script>

{#if device === "mobile"}
  <Smartphone {size} strokeWidth={1.75} />
{:else if device === "tablet"}
  <Tablet {size} strokeWidth={1.75} />
{:else if device === "desktop"}
  <Monitor {size} strokeWidth={1.75} />
{:else}
  <CircleHelp {size} strokeWidth={1.75} />
{/if}
