<script lang="ts">
  import RotateCw from "@lucide/svelte/icons/rotate-cw";
  import RedeployGroups from "./RedeployGroups.svelte";
  import { formatCount } from "../../utils/format";
  import type { RedeployGroup } from "../../utils/hosts";

  let {
    groups,
    count,
    hostName,
    checked,
  }: { groups: RedeployGroup[]; count: number; hostName: string; checked: string } = $props();
</script>

<div class="card redeploy-banner">
  <div class="banner-head">
    <span class="banner-icon" aria-hidden="true"><RotateCw size={18} strokeWidth={2} /></span>
    <div class="banner-text">
      <strong>{formatCount(count, "container")} {count === 1 ? "needs" : "need"} a redeploy</strong>
      <span class="muted">Copy a stack's command and run it on {hostName}.</span>
    </div>
    <span class="muted checked">{checked}</span>
  </div>

  <div class="banner-groups"><RedeployGroups {groups} /></div>
</div>

<style>
  .redeploy-banner {
    margin-bottom: var(--space-4);
    padding: var(--space-4) var(--space-5);
  }

  .banner-head {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .banner-icon {
    display: inline-flex;
    flex-shrink: 0;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    border-radius: 50%;
    background: var(--color-warning-bg);
    color: var(--color-warning);
  }

  .banner-text {
    display: flex;
    flex: 1;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
    font-size: 0.875rem;
  }

  .banner-text strong {
    font-size: 0.9375rem;
  }

  .checked {
    align-self: flex-start;
    font-size: 0.75rem;
    white-space: nowrap;
  }

  .banner-groups {
    margin-top: var(--space-3);
  }

  @media (max-width: 640px) {
    .banner-head {
      flex-wrap: wrap;
    }
  }
</style>
