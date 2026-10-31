<script lang="ts">
  import Check from "@lucide/svelte/icons/check";
  import Undo2 from "@lucide/svelte/icons/undo-2";
  import Button from "./Button.svelte";
  import { allCount } from "../utils/format";

  let {
    from,
    to,
    groupSize = 0,
    disabled = false,
    onRevert,
  }: {
    from: string;
    to: string;
    groupSize?: number;
    disabled?: boolean;
    onRevert: () => void;
  } = $props();
</script>

<p class="notice">
  <Check size={14} strokeWidth={2.5} />
  <span class="notice-text">
    {groupSize > 0 ? `${groupSize === 2 ? "Both" : `All ${groupSize}`} images updated` : "Updated"} from
    <strong>{from}</strong> to
    <strong>{to}</strong>
  </span>
  <Button variant="secondary" size="sm" {disabled} onclick={onRevert}>
    <Undo2 size={14} strokeWidth={2} />
    {groupSize > 0 ? `Revert ${allCount(groupSize)}` : "Revert"}
  </Button>
</p>
