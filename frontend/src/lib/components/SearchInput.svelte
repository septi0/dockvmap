<script lang="ts">
  import Search from "@lucide/svelte/icons/search";
  import { debounce } from "../utils/debounce";

  let {
    value = $bindable(""),
    placeholder = "Search…",
    onSearch,
    delay = 300,
  }: {
    value?: string;
    placeholder?: string;
    onSearch: (value: string) => void;
    delay?: number;
  } = $props();

  let debouncedSearch = $derived(debounce(onSearch, delay));

  function handleInput() {
    debouncedSearch(value);
  }
</script>

<div class="search-input">
  <span class="icon"><Search size={16} strokeWidth={1.75} /></span>
  <input
    type="search"
    class="input"
    {placeholder}
    bind:value
    oninput={handleInput}
  />
</div>

<style>
  .search-input {
    position: relative;
    display: flex;
    align-items: center;
    width: 100%;
    max-width: 280px;
  }

  .icon {
    position: absolute;
    left: var(--space-3);
    display: inline-flex;
    color: var(--color-text-faint);
    pointer-events: none;
  }

  .search-input .input {
    padding-left: calc(var(--space-3) * 2 + 16px);
  }
</style>
