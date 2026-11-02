<script lang="ts">
  import StateIndicator from "../StateIndicator.svelte";
  import { formatDate } from "../../utils/format";
  import { containerState } from "../../utils/hosts";
  import type { HostContainer } from "../../api/types/hosts";

  let { container }: { container: HostContainer } = $props();

  let view = $derived(containerState(container));

  let title = $derived(
    container.state === "running" && container.startedAt
      ? `Started ${formatDate(container.startedAt)}`
      : container.finishedAt
        ? `Stopped ${formatDate(container.finishedAt)}`
        : undefined,
  );
</script>

<StateIndicator tone={view.tone} label={view.label} detail={view.detail} {title} inline />
