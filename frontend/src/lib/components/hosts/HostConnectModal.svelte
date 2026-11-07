<script lang="ts">
  import { onDestroy } from "svelte";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import CircleCheck from "@lucide/svelte/icons/circle-check";
  import Modal from "../Modal.svelte";
  import Button from "../Button.svelte";
  import CopyButton from "../CopyButton.svelte";
  import RefreshingIndicator from "../RefreshingIndicator.svelte";
  import { getHost } from "../../api/hosts";
  import { getVersion } from "../../api/system";
  import { createPoller } from "../../utils/poller";
  import { agentCompose } from "../../utils/hosts";
  import type { Host, HostConnection } from "../../api/types/hosts";

  const CHECK_INTERVAL_MS = 2000;

  let {
    connection,
    onClose,
  }: {
    connection: HostConnection | null;
    onClose: () => void;
  } = $props();

  let appVersion = $state("");
  let host = $state<Host | null>(null);

  let connected = $derived(
    !!host &&
      (host.agentUpdateNeeded || host.lastSeenAt !== connection?.lastSeenAt),
  );

  let address = $derived(host?.address ?? connection?.address ?? "");
  let compose = $derived(connection ? agentCompose(address, appVersion) : "");

  const poller = createPoller(async () => {
    if (!connection) return false;

    try {
      host = (await getHost(connection.id)).host;
    } catch {
      return true;
    }

    return !connected;
  }, CHECK_INTERVAL_MS);

  $effect(() => {
    if (!connection) return;

    host = null;
    poller.start();
    getVersion()
      .then((v) => (appVersion = v.version))
      .catch(() => {});

    return () => poller.stop();
  });

  onDestroy(() => poller.stop());
</script>

<Modal open={connection !== null} {onClose} title="Connect {connection?.name ?? ''}" size="lg">
  {#if connection}
    <h3>Run the agent on {connection.name}</h3>
    <p class="section-hint muted">
      Save this as <code class="inline-code">compose.yaml</code> on the host and
      run <code class="inline-code">docker compose up -d</code>. DockVMap
      connects to it at <code class="inline-code">{address}</code>, so allow
      that port from DockVMap in the host's firewall. The agent keeps its
      identity in the <code class="inline-code">dockvmap-agent</code> volume.
      Other options, like a read-only socket proxy, are in the
      <a
        class="link"
        href="https://github.com/septi0/dockvmap#hosts-and-the-agent"
        target="_blank"
        rel="noopener noreferrer">README</a
      >.
    </p>

    <div class="command-row">
      <code class="command">{compose}</code>
      <CopyButton
        text={compose}
        label="Copy compose file"
        copiedMessage="Compose file copied to clipboard."
        bordered
      />
    </div>

    <div class="connection-status">
      {#if !connected || !host}
        <RefreshingIndicator text="Waiting for DockVMap to reach the agent…" />
        {#if host?.pollError}
          <p class="muted last-try">Last try: {host.pollError}</p>
        {/if}
      {:else if host.agentUpdateNeeded}
        <p class="warning-text">
          <TriangleAlert size={16} strokeWidth={2} />
          The agent connected, but its version doesn't work with this DockVMap
          version. Run the same version on both.
        </p>
      {:else if host.dockerError}
        <p class="warning-text">
          <TriangleAlert size={16} strokeWidth={2} />
          Connected, but: {host.dockerError}
        </p>
      {:else}
        <p class="success-text">
          <CircleCheck size={16} strokeWidth={2} />
          Connected. Found {host.runningContainers} running and {host.stoppedContainers}
          stopped containers.
        </p>
      {/if}
    </div>

    <div class="modal-actions spaced">
      <Button onclick={onClose}>Done</Button>
    </div>
  {/if}
</Modal>

<style>
  h3 {
    margin: 0 0 var(--space-2);
    font-size: 0.9375rem;
  }

  .last-try {
    margin: var(--space-2) 0 0;
    font-size: 0.8125rem;
    overflow-wrap: anywhere;
  }

  .connection-status {
    margin-top: var(--space-4);
  }

  .success-text {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    margin: 0;
    color: var(--color-success);
    font-size: 0.8125rem;
  }
</style>
