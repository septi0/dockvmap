<script lang="ts">
  import StateIndicator from "../StateIndicator.svelte";
  import { formatDate, formatRelativeTime } from "../../utils/format";
  import type { Host } from "../../api/types/hosts";
  import type { StateTone } from "../../utils/hosts";

  let {
    host,
    showLastSeen = false,
    inline = false,
  }: { host: Host; showLastSeen?: boolean; inline?: boolean } = $props();

  let view = $derived.by((): { label: string; tone: StateTone; title?: string } => {
    if (host.agentUpdateNeeded) {
      return {
        label: "Update agent",
        tone: "warning",
        title: "This agent version doesn't work with this DockVMap version. Run the same version on both.",
      };
    }

    if (!host.lastSeenAt) {
      return { label: "Waiting for agent", tone: "stopped", title: "Run the agent on this host with its token." };
    }

    if (!host.online) {
      return { label: "Offline", tone: "failed" };
    }

    if (host.dockerError) {
      return { label: "Can't read Docker", tone: "warning", title: host.dockerError };
    }

    return { label: "Online", tone: "running" };
  });

  let lastSeen = $derived(
    showLastSeen && !host.current
      ? host.lastSeenAt
        ? `last seen ${formatRelativeTime(host.lastSeenAt)}`
        : "never connected"
      : undefined,
  );
</script>

<StateIndicator
  tone={view.tone}
  label={view.label}
  title={view.title}
  detail={lastSeen}
  detailTitle={host.lastSeenAt ? formatDate(host.lastSeenAt) : undefined}
  {inline}
/>
