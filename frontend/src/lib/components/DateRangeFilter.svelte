<script lang="ts">
  let {
    since = $bindable(""),
    until = $bindable(""),
    onChange,
  }: {
    since?: string;
    until?: string;
    onChange: () => void;
  } = $props();
</script>

<div class="date-range">
  <input
    type="date"
    class="input filter-control"
    class:is-active={since !== ""}
    aria-label="From date"
    bind:value={since}
    onchange={onChange}
  />
  <span class="sep">–</span>
  <input
    type="date"
    class="input filter-control"
    class:is-active={until !== ""}
    aria-label="To date"
    bind:value={until}
    onchange={onChange}
  />
</div>

<style>
  .date-range {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .date-range .input {
    width: 152px;
  }

  .sep {
    color: var(--color-text-faint);
  }
</style>
