<script lang="ts">
  import { onMount } from "svelte";
  import Play from "@lucide/svelte/icons/play";
  import AsyncState from "../AsyncState.svelte";
  import Button from "../Button.svelte";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import { getSystemTasks, runSystemTask } from "../../api/system";
  import { errorMessage } from "../../api/client";
  import { toast } from "../../services/toast";
  import type { SystemTask } from "../../api/types/system";
  import {
    formatDate,
    formatDuration,
    formatRelativeTime,
  } from "../../utils/format";

  let tasks = $state<SystemTask[]>([]);
  let loading = $state(true);
  let loaded = $state(false);
  let error = $state<string | null>(null);
  let actionError = $state<string | null>(null);
  let running = $state<Set<string>>(new Set());

  onMount(load);

  async function load() {
    loading = true;
    error = null;

    try {
      tasks = (await getSystemTasks()).tasks;
    } catch (err) {
      error = errorMessage(err, "Failed to load background tasks");
    } finally {
      loading = false;
      loaded = true;
    }
  }

  async function triggerTask(task: SystemTask) {
    actionError = null;
    running = new Set(running).add(task.name);

    try {
      await runSystemTask(task.name);
      toast.success(`Triggered ${task.name}.`);
      setTimeout(load, 2000);
    } catch (err) {
      actionError = errorMessage(err, `Failed to trigger ${task.name}`);
    } finally {
      const next = new Set(running);
      next.delete(task.name);
      running = next;
    }
  }
</script>

{#if actionError}
  <p class="error">
    <TriangleAlert size={16} strokeWidth={2} />
    {actionError}
  </p>
{/if}

<AsyncState
  loading={loading && !loaded}
  busy={loading && loaded}
  {error}
  empty={tasks.length === 0}
  emptyMessage="No tasks."
  columns={4}
>
  <div class="card table-wrap">
    <table class="table">
      <thead>
        <tr>
          <th>Task</th>
          <th>Schedule</th>
          <th>Last run</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        {#each tasks as task (task.name)}
          <tr class:is-disabled={!task.enabled}>
            <td>
              <div class="task-name">
                <span class="mono">{task.name}</span>
                {#if task.running}
                  <span class="badge badge-accent">Running</span>
                {/if}
              </div>
              <div class="task-desc muted">{task.description}</div>
            </td>
            <td>
              {#if task.enabled}
                <div>Every {formatDuration(task.intervalSeconds)}</div>
                {#if task.nextDue}
                  <div class="muted small">
                    next {formatRelativeTime(task.nextDue)}
                  </div>
                {/if}
              {:else}
                <span class="muted">Disabled: {task.disabledReason}</span>
              {/if}
            </td>
            <td>
              {#if task.lastRun}
                <div title={formatDate(task.lastRun)}>
                  {formatRelativeTime(task.lastRun)}
                </div>
                {#if task.lastError}
                  <div class="last-error" title={task.lastError}>
                    {task.lastError}
                  </div>
                {:else if task.lastCount != null && task.lastCount > 0}
                  <div class="muted small">{task.lastCount} affected</div>
                {:else}
                  <div class="muted small">ok</div>
                {/if}
              {:else}
                <span class="muted">never</span>
              {/if}
            </td>
            <td class="actions-cell">
              {#if task.enabled && task.triggerable}
                <Button
                  variant="secondary"
                  size="sm"
                  disabled={running.has(task.name) || task.running}
                  onclick={() => triggerTask(task)}
                >
                  <Play size={13} strokeWidth={2} />
                  Run now
                </Button>
              {/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
</AsyncState>

<style>
  .task-name {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .task-desc {
    margin-top: 2px;
    font-size: 0.8125rem;
  }

  .mono {
    font-family: ui-monospace, monospace;
    font-size: 0.8125rem;
    font-weight: 500;
  }

  .small {
    font-size: 0.75rem;
  }

  .last-error {
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    overflow: hidden;
    overflow-wrap: anywhere;
    font-size: 0.75rem;
    color: var(--color-danger);
  }

  .actions-cell {
    text-align: right;
    white-space: nowrap;
  }

  tr.is-disabled .task-name .mono,
  tr.is-disabled .task-desc {
    opacity: 0.65;
  }
</style>
