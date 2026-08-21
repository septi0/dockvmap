<script lang="ts">
  import Eye from "@lucide/svelte/icons/eye";
  import EyeOff from "@lucide/svelte/icons/eye-off";

  let {
    label,
    type = "text",
    value = $bindable(""),
    autocomplete,
    placeholder,
    required = false,
  }: {
    label: string;
    type?: string;
    value?: string;
    autocomplete?: HTMLInputElement["autocomplete"];
    placeholder?: string;
    required?: boolean;
  } = $props();

  let revealed = $state(false);
  let isPassword = $derived(type === "password");
  let inputType = $derived(isPassword && revealed ? "text" : type);
</script>

<label class="field">
  <span class="field-label">{label}</span>
  <div class="input-wrap">
    <input
      class="input"
      type={inputType}
      bind:value
      {autocomplete}
      {placeholder}
      {required}
    />
    {#if isPassword}
      <button
        type="button"
        class="toggle"
        onclick={() => (revealed = !revealed)}
        aria-label={revealed ? "Hide password" : "Show password"}
      >
        {#if revealed}
          <EyeOff size={16} strokeWidth={1.75} />
        {:else}
          <Eye size={16} strokeWidth={1.75} />
        {/if}
      </button>
    {/if}
  </div>
</label>

<style>
  .input-wrap {
    position: relative;
    display: flex;
  }

  .input-wrap .input {
    padding-right: calc(var(--space-3) * 2 + 16px);
  }

  .toggle {
    position: absolute;
    right: var(--space-2);
    top: 50%;
    transform: translateY(-50%);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: none;
    border: none;
    padding: var(--space-1);
    color: var(--color-text-faint);
    cursor: pointer;
  }

  .toggle:hover {
    color: var(--color-text);
  }
</style>
