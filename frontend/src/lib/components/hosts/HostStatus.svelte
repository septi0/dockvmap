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

    if (host.pollErrorKind === "identityChanged") {
      return {
        label: "Agent identity changed",
        tone: "failed",
        title: `The agent at ${host.address} isn't the one DockVMap adopted. If you reset or reinstalled it, trust the new agent on this host's page.`,
      };
    }

    if (host.pollErrorKind === "pairedElsewhere") {
      return {
        label: "Paired with another DockVMap",
        tone: "failed",
        title: `The agent at ${host.address} belongs to another DockVMap. Delete its data volume and start it again to pair it with this one.`,
      };
    }

    if (!host.lastSeenAt) {
      return {
        label: "Waiting for agent",
        tone: "stopped",
        title: host.pollError
          ? `Can't reach the agent at ${host.address}: ${host.pollError}`
          : `Start the agent on this host. DockVMap adopts it at ${host.address}.`,
      };
    }

    if (!host.online) {
      return {
        label: "Offline",
        tone: "failed",
        title: host.pollError ? `Can't reach the agent at ${host.address}: ${host.pollError}` : undefined,
      };
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
