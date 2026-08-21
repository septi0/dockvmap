<script lang="ts">
  let {
    label,
    value = $bindable(""),
    options,
    placeholder,
    required = false,
  }: {
    label: string;
    value?: string;
    options: { value: string; label: string }[];
    placeholder?: string;
    required?: boolean;
  } = $props();
</script>

<label class="field">
  <span class="field-label">{label}</span>
  <select class="input" bind:value {required}>
    {#if placeholder}
      <option value="" disabled selected={value === ""}>{placeholder}</option>
    {/if}
    {#each options as option (option.value)}
      <option value={option.value}>{option.label}</option>
    {/each}
  </select>
</label>
