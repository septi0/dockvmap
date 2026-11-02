<script lang="ts">
  import type { StateTone } from "../utils/hosts";

  let {
    tone,
    label,
    detail,
    title,
    detailTitle,
    inline = false,
  }: {
    tone: StateTone;
    label: string;
    detail?: string;
    title?: string;
    detailTitle?: string;
    inline?: boolean;
  } = $props();
</script>

<span class="state-indicator" class:inline {title}>
  <span class="state-label">
    <span class="state-dot {tone}" aria-hidden="true"></span>
    {label}
  </span>
  {#if detail}
    <span class="muted state-detail" title={detailTitle}>{detail}</span>
  {/if}
</span>

<style>
  .state-indicator {
    display: inline-flex;
    flex-direction: column;
    gap: 2px;
  }

  .state-label {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    white-space: nowrap;
  }

  .state-detail {
    padding-left: calc(8px + var(--space-2));
    font-size: 0.8125rem;
  }

  @media (min-width: 1100px) {
    .inline {
      flex-direction: row;
      align-items: center;
      gap: var(--space-1);
      white-space: nowrap;
    }

    .inline .state-detail {
      padding-left: 0;
    }

    .inline .state-detail::before {
      content: "· ";
    }
  }
</style>
