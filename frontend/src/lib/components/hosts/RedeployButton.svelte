<script lang="ts">
  import CopyButton from "../CopyButton.svelte";
  import type { HostContainer } from "../../api/types/hosts";

  let { container, command }: { container: HostContainer; command: string } = $props();
</script>

<CopyButton
  text={command}
  label={container.tracking?.status === "outdated"
    ? `Copy the command that pulls and recreates ${container.name}`
    : `Copy the command that recreates ${container.name}`}
  copiedMessage="Command copied to clipboard."
/>
