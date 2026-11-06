<script lang="ts">
  import CopyButton from "../CopyButton.svelte";
  import type { RedeployGroup } from "../../utils/hosts";

  let { groups, showHost = false }: { groups: RedeployGroup[]; showHost?: boolean } = $props();
</script>

<ul class="groups">
  {#each groups as group (group.key)}
    <li class="group">
      <div class="group-text">
        <span class="group-name">
          {group.name}{#if showHost}<span class="muted group-host">{` on ${group.hostName}`}</span>{/if}
        </span>
        <span class="muted group-detail">{group.detail}</span>
      </div>
      {#if group.command}
        <CopyButton
          text={group.command}
          label={group.commandTitle}
          buttonText="Copy command"
          copiedMessage="Command copied. Run it on {group.hostName}."
        />
      {/if}
    </li>
  {/each}
</ul>

<style>
  .groups {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .group {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2) var(--space-3);
    padding: var(--space-3) 0;
    border-top: 1px solid var(--color-border);
  }

  .group-text {
    display: flex;
    flex: 1 1 200px;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .group-name {
    font-size: 0.875rem;
    font-weight: 600;
    overflow-wrap: anywhere;
  }

  .group-host {
    font-weight: 400;
  }

  .group-detail {
    font-size: 0.8125rem;
    overflow-wrap: anywhere;
  }
</style>
