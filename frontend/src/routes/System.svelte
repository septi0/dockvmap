<script lang="ts">
  import { replace, link } from "svelte-spa-router";
  import active from "svelte-spa-router/active";
  import Cog from "@lucide/svelte/icons/cog";
  import AppShell from "../lib/components/AppShell.svelte";
  import PageTitle from "../lib/components/PageTitle.svelte";
  import StatusTab from "../lib/components/system/StatusTab.svelte";
  import TasksTab from "../lib/components/system/TasksTab.svelte";
  import FailuresTab from "../lib/components/system/FailuresTab.svelte";
  import AuditTab from "../lib/components/system/AuditTab.svelte";

  let { params }: { params?: { tab?: string } } = $props();

  const TABS = [
    { id: "status", label: "Status" },
    { id: "tasks", label: "Tasks" },
    { id: "failures", label: "Failures" },
    { id: "audit", label: "Audit log" },
  ] as const;

  let rawTab = $derived(params?.tab);
  let tab = $derived(rawTab ?? "status");

  $effect(() => {
    if (!TABS.some((t) => t.id === rawTab)) {
      replace("/system/status");
    }
  });
</script>

<PageTitle title="System" />

<AppShell>
  <div class="title-row">
    <Cog size={20} strokeWidth={1.75} />
    <h1>System</h1>
  </div>

  <nav class="tabs">
    {#each TABS as t (t.id)}
      <a
        href={`/system/${t.id}`}
        use:link
        use:active={{ path: `/system/${t.id}`, className: "active" }}
      >
        {t.label}
      </a>
    {/each}
  </nav>

  {#if tab === "status"}
    <StatusTab />
  {:else if tab === "tasks"}
    <TasksTab />
  {:else if tab === "failures"}
    <FailuresTab />
  {:else if tab === "audit"}
    <AuditTab />
  {/if}
</AppShell>

<style>
  .title-row {
    margin-bottom: var(--space-4);
  }

  .tabs {
    display: flex;
    gap: var(--space-1);
    border-bottom: 1px solid var(--color-border);
    margin-bottom: var(--space-5);
  }

  .tabs a {
    padding: var(--space-2) var(--space-3);
    font-size: 0.875rem;
    font-weight: 500;
    color: var(--color-text-muted);
    text-decoration: none;
    border-bottom: 2px solid transparent;
    margin-bottom: -1px;
    transition:
      color var(--transition-fast),
      border-color var(--transition-fast);
  }

  .tabs a:hover {
    color: var(--color-text);
  }

  .tabs a:global(.active) {
    color: var(--color-accent);
    border-bottom-color: var(--color-accent);
  }
</style>
