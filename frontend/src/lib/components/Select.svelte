<script lang="ts">
  let {
    label,
    value = $bindable(""),
    options,
    placeholder,
    required = false,
    disabled = false,
    onchange,
  }: {
    label: string;
    value?: string;
    options: { value: string; label: string }[];
    placeholder?: string;
    required?: boolean;
    disabled?: boolean;
    onchange?: () => void;
  } = $props();
</script>

<label class="field">
  <span class="field-label">{label}</span>
  <select class="input" bind:value {required} {disabled} {onchange}>
    {#if placeholder}
      <option value="" disabled selected={value === ""}>{placeholder}</option>
    {/if}
    {#each options as option (option.value)}
      <option value={option.value}>{option.label}</option>
    {/each}
  </select>
</label>
