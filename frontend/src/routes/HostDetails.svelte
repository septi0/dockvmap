<script lang="ts">
  import { link, push } from "svelte-spa-router";
  import ArrowLeft from "@lucide/svelte/icons/arrow-left";
  import HardDrive from "@lucide/svelte/icons/hard-drive";
  import KeyRound from "@lucide/svelte/icons/key-round";
  import Layers from "@lucide/svelte/icons/layers";
  import Trash2 from "@lucide/svelte/icons/trash-2";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import AppShell from "../lib/components/AppShell.svelte";
  import PageTitle from "../lib/components/PageTitle.svelte";
  import AsyncState from "../lib/components/AsyncState.svelte";
  import Button from "../lib/components/Button.svelte";
  import ConfirmDialog from "../lib/components/ConfirmDialog.svelte";
  import SearchInput from "../lib/components/SearchInput.svelte";
  import FilterBar from "../lib/components/FilterBar.svelte";
  import HostStatus from "../lib/components/hosts/HostStatus.svelte";
  import HostConnectModal from "../lib/components/hosts/HostConnectModal.svelte";
  import ContainerState from "../lib/components/hosts/ContainerState.svelte";
  import { deleteHost, getHost, regenerateHostToken } from "../lib/api/hosts";
  import { errorMessage } from "../lib/api/client";
  import { toast } from "../lib/services/toast";
  import { capitalize, formatBytes, formatDate, formatRelativeTime, imageParts } from "../lib/utils/format";
  import { formatPorts } from "../lib/utils/hosts";
  import { createVisibilityPoller } from "../lib/utils/visibilityPoller";
  import type { HostConnection, HostContainer, HostDetails } from "../lib/api/types/hosts";

  const REFRESH_INTERVAL_MS = 15_000;
  const NOT_IN_STACK = "-";

  const SHOW_OPTIONS: {
    value: string;
    label: string;
    matches: (c: HostContainer) => boolean;
  }[] = [
    { value: "", label: "All containers", matches: () => true },
    { value: "running", label: "Running", matches: (c) => c.state === "running" },
    { value: "stopped", label: "Stopped", matches: (c) => c.state !== "running" },
    { value: "dockvmap", label: "Via DockVMap proxy", matches: (c) => !!c.tracking },
    { value: "outdated", label: "Outdated", matches: (c) => !!c.tracking?.needsUpdate },
    { value: "crashed", label: "Crashed", matches: (c) => c.recentlyCrashed },
  ];

  let { params }: { params: { id: string } } = $props();
  const hostId = $derived(Number(params.id));

  let details = $state<HostDetails | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let search = $state("");
  let show = $state("");
  let stack = $state("");
  let showTokenConfirm = $state(false);
  let showDeleteConfirm = $state(false);
  let actionError = $state<string | null>(null);
  let submitting = $state(false);
  let connection = $state<HostConnection | null>(null);
  let loadToken = 0;

  let host = $derived(details?.host ?? null);
  let hasContainers = $derived((details?.containers?.length ?? 0) > 0);

  let stackSizes = $derived.by(() => {
    const sizes = new Map<string, number>();

    for (const c of details?.containers ?? []) {
      const name = c.composeProject ?? NOT_IN_STACK;
      sizes.set(name, (sizes.get(name) ?? 0) + 1);
    }

    return sizes;
  });

  let stacks = $derived([...stackSizes.keys()].filter((name) => name !== NOT_IN_STACK).sort());

  let hostFacts = $derived(
    host
      ? [
          host.info.hostname !== host.name ? host.info.hostname : null,
          host.info.os,
          host.info.dockerVersion ? `Docker ${host.info.dockerVersion}` : null,
          host.info.architecture,
          host.info.cpus ? `${host.info.cpus} CPUs` : null,
          host.info.memoryBytes ? formatBytes(host.info.memoryBytes) : null,
          host.agentVersion ? `agent ${host.agentVersion}` : null,
        ].filter((fact): fact is string => !!fact)
      : [],
  );

  let containers = $derived.by(() => {
    const term = search.trim().toLowerCase();
    const matchesShow = SHOW_OPTIONS.find((o) => o.value === show)?.matches ?? (() => true);

    return (details?.containers ?? [])
      .filter(
        (c) =>
          matchesShow(c) &&
          (!stack || (c.composeProject ?? NOT_IN_STACK) === stack) &&
          (!term ||
            [c.name, c.image, c.composeProject, c.tracking?.imageName].some((field) =>
              field?.toLowerCase().includes(term),
            )),
      )
      .sort((a, b) => a.name.localeCompare(b.name));
  });

  let filtersActive = $derived(!!search || !!show || !!stack);

  function clearFilters() {
    search = "";
    show = "";
    stack = "";
  }

  function cancelAction() {
    showTokenConfirm = false;
    showDeleteConfirm = false;
    actionError = null;
  }

  async function handleRegenerate() {
    if (!host) return;

    const { id, name, lastSeenAt } = host;
    actionError = null;
    submitting = true;

    try {
      const { token } = await regenerateHostToken(id);
      const fresh = await getHost(id).catch(() => null);
      showTokenConfirm = false;
      connection = { id, name, token, lastSeenAt: fresh ? fresh.host.lastSeenAt : lastSeenAt };
    } catch (err) {
      actionError = errorMessage(err, "Failed to regenerate the token");
    } finally {
      submitting = false;
    }
  }

  async function handleDelete() {
    if (!host) return;

    const { id, name } = host;
    actionError = null;
    submitting = true;

    try {
      await deleteHost(id);
      toast.success(`Host "${name}" deleted.`);
      push("/hosts");
    } catch (err) {
      actionError = errorMessage(err, "Failed to delete host");
    } finally {
      submitting = false;
    }
  }

  function closeConnection() {
    connection = null;
    load();
  }

  async function load() {
    const requestId = ++loadToken;
    error = null;

    try {
      const result = await getHost(hostId);
      if (requestId !== loadToken) return;
      details = result;
    } catch (err) {
      if (requestId !== loadToken) return;
      error = errorMessage(err, "Failed to load the host");
    } finally {
      if (requestId === loadToken) loading = false;
    }
  }

  $effect(() => {
    hostId;
    details = null;
    loading = true;

    return createVisibilityPoller({
      tick: load,
      intervalMs: REFRESH_INTERVAL_MS,
    }).start();
  });
</script>

{#snippet imageName(container: HostContainer)}
  {@const shown = container.tracking ? container.image.slice(container.image.indexOf("/") + 1) : container.image}
  {#each imageParts(shown) as part, index (index)}{part}<wbr />{/each}
{/snippet}

{#snippet imageInfo(container: HostContainer)}
  {@const tracking = container.tracking}
  <div class="image-cell" class:proxied={tracking?.imageId}>
    {#if tracking?.imageId}
      <a
        class="row-link"
        href={`/images/${tracking.imageId}`}
        title="{container.image}, pulled through the DockVMap proxy. Open the virtual image {tracking.imageName}."
        use:link>{@render imageName(container)}</a
      >
    {:else}
      <span title={tracking ? `${container.image}, pulled through the DockVMap proxy` : undefined}
        >{@render imageName(container)}</span
      >
    {/if}
    {#if host?.current && tracking?.needsUpdate}
      {@const warning = tracking.status === "outdated"
        ? "Runs an older version than DockVMap points to."
        : "Pulled, not recreated. Recreate the container to apply it."}
      <span class="version-warning" title={warning} aria-label={warning}>
        <TriangleAlert size={14} strokeWidth={2} />
      </span>
    {/if}
  </div>
{/snippet}

<PageTitle title={host ? host.name : "Host"} />

<AppShell>
  <a class="back-link" href="/hosts" use:link>
    <ArrowLeft size={14} strokeWidth={2} />
    Hosts
  </a>

  <AsyncState loading={loading && !details} {error}>
    {#if host && details}
      <div class="page-header">
        <div class="title-row">
          <HardDrive size={20} strokeWidth={1.75} />
          <h1>{host.name}</h1>
          <span class="host-status"><HostStatus {host} /></span>
        </div>
        <p class="subtitle host-facts">
          {#if host.lastSeenAt}
            <span title={formatDate(host.lastSeenAt)}
              >last seen {formatRelativeTime(host.lastSeenAt)}</span
            >
          {:else}
            never connected
          {/if}
          {#if hostFacts.length > 0}· {hostFacts.join(" · ")}{/if}
        </p>
      </div>

      {#if host.agentUpdateNeeded}
        <p class="notice warning">
          <TriangleAlert size={16} strokeWidth={2} />
          <span class="notice-text"
            >This agent version doesn't work with this DockVMap version. Run the
            same version on both.
            {#if hasContainers}This list is from when it was last online.{/if}</span
          >
        </p>
      {:else if !host.lastSeenAt}
        <p class="notice">
          <span class="notice-text"
            >The agent hasn't connected yet. Run it on this host with its token,
            or regenerate the token below if you no longer have it.</span
          >
        </p>
      {:else if !host.online}
        <p class="notice warning">
          <TriangleAlert size={16} strokeWidth={2} />
          <span class="notice-text"
            >This host went offline {formatRelativeTime(host.lastSeenAt)}.
            {#if hasContainers}This list is from when it was last online.{/if}</span
          >
        </p>
      {:else if host.dockerError}
        <p class="notice warning">
          <TriangleAlert size={16} strokeWidth={2} />
          <span class="notice-text"
            >{capitalize(host.dockerError)}
            {#if hasContainers}<br />This list is from the last time the agent could read Docker.{/if}</span
          >
        </p>
      {/if}

      {#if host.lastSeenAt}
        <div class="card section-card">
          <h2 class="heading-with-count">
            Containers <span class="muted heading-count"
              >{host.runningContainers} running, {host.stoppedContainers} stopped</span
            >
          </h2>

          <FilterBar active={filtersActive} onClear={clearFilters}>
            <SearchInput
              bind:value={search}
              placeholder="Search containers…"
              onSearch={() => {}}
              delay={0}
            />
            <div class="filter-field">
              <span class="filter-label">Show</span>
              <select
                class="input filter-control"
                class:is-active={show !== ""}
                bind:value={show}
              >
                {#each SHOW_OPTIONS as option (option.value)}
                  <option value={option.value}>{option.label}</option>
                {/each}
              </select>
            </div>
            {#if stacks.length > 0}
              <div class="filter-field">
                <span class="filter-label">Stack</span>
                <select
                  class="input filter-control"
                  class:is-active={stack !== ""}
                  bind:value={stack}
                >
                  <option value="">All stacks</option>
                  {#if stackSizes.has(NOT_IN_STACK)}
                    <option value={NOT_IN_STACK}
                      >Not in a stack ({stackSizes.get(NOT_IN_STACK)})</option
                    >
                  {/if}
                  {#each stacks as name (name)}
                    <option value={name}>{name} ({stackSizes.get(name)})</option>
                  {/each}
                </select>
              </div>
            {/if}
          </FilterBar>

          {#if containers.length === 0}
            <p class="muted">
              {filtersActive
                ? "No containers match these filters."
                : host.current
                  ? "No containers on this host."
                  : "No containers known yet."}
            </p>
          {:else}
            <div class="table-wrap">
              <table class="table">
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>State</th>
                    <th class="phone-hidden">Image</th>
                    <th class="phone-hidden ports-cell">Ports</th>
                  </tr>
                </thead>
                <tbody>
                  {#each containers as container (container.id)}
                    <tr>
                      <td>
                        <span class="name-cell">
                          <span class="container-name">{container.name}</span>
                          {#if container.composeProject}
                            {@const project = container.composeProject}
                            <button
                              type="button"
                              class="group-label"
                              title={`Part of the ${project} stack. Click to show only this stack.`}
                              onclick={() => (stack = project)}
                            >
                              <Layers size={12} strokeWidth={2} />
                              <span class="group-label-name">{project}</span>
                            </button>
                          {/if}
                        </span>
                        <div class="phone-only image-info">
                          {@render imageInfo(container)}
                        </div>
                      </td>
                      <td><ContainerState {container} /></td>
                      <td class="phone-hidden">{@render imageInfo(container)}</td>
                      <td class="phone-hidden ports-cell">
                        {#each formatPorts(container.ports) as port (port)}
                          <span class="port">{port}</span>
                        {/each}
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
        </div>
      {/if}

      <div class="card section-card danger-card">
        <h2>Danger zone</h2>

        <div class="danger-action">
          <div class="danger-text">
            <strong>Regenerate token</strong>
            <p class="muted">
              The current token stops working right away, and the agent goes
              offline until it runs with the new one.
            </p>
          </div>
          <Button variant="secondary" size="sm" onclick={() => (showTokenConfirm = true)}>
            <KeyRound size={14} strokeWidth={2} />
            Regenerate
          </Button>
        </div>

        <div class="danger-action">
          <div class="danger-text">
            <strong>Delete host</strong>
            <p class="muted">
              Its token stops working and it disappears from DockVMap. The agent
              keeps running until you remove it on the host.
            </p>
          </div>
          <Button variant="danger" size="sm" onclick={() => (showDeleteConfirm = true)}>
            <Trash2 size={14} strokeWidth={2} />
            Delete
          </Button>
        </div>
      </div>
    {/if}
  </AsyncState>
</AppShell>

<ConfirmDialog
  open={showTokenConfirm}
  title="Regenerate token"
  message={`Regenerate the token for "${host?.name ?? ""}"? The current token stops working immediately, so the agent on this host goes offline until it runs with the new one.`}
  confirmLabel="Regenerate"
  error={actionError}
  {submitting}
  onConfirm={handleRegenerate}
  onCancel={cancelAction}
/>

<ConfirmDialog
  open={showDeleteConfirm}
  title="Delete host"
  message={`Delete "${host?.name ?? ""}"? Its token stops working and its containers disappear from DockVMap. The agent on the host keeps running until you remove it there.`}
  confirmLabel="Delete"
  danger
  error={actionError}
  {submitting}
  onConfirm={handleDelete}
  onCancel={cancelAction}
/>

<HostConnectModal {connection} onClose={closeConnection} />

<style>
  .host-status {
    margin-left: var(--space-2);
    font-size: 0.875rem;
  }

  .host-facts {
    font-size: 0.875rem;
  }

  .notice {
    margin-bottom: var(--space-4);
  }

  .container-name {
    font-weight: 500;
  }

  .image-info {
    margin-top: var(--space-1);
  }

  .image-cell {
    margin-bottom: 2px;
    font-family: ui-monospace, monospace;
    font-size: 0.8125rem;
    color: var(--color-text-muted);
    overflow-wrap: break-word;
  }

  .image-cell.proxied {
    color: var(--color-accent);
  }

  .version-warning {
    display: inline-flex;
    margin-left: var(--space-1);
    color: var(--color-warning);
    vertical-align: -2px;
  }

  .ports-cell {
    width: 1%;
  }

  .port {
    display: block;
    font-family: ui-monospace, monospace;
    font-size: 0.8125rem;
    white-space: nowrap;
  }
</style>
