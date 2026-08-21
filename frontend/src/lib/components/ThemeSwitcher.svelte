<script lang="ts">
  import Sun from "@lucide/svelte/icons/sun";
  import Moon from "@lucide/svelte/icons/moon";
  import Monitor from "@lucide/svelte/icons/monitor";
  import { theme } from "../services/theme";
</script>

<div class="theme-switcher" role="radiogroup" aria-label="Theme">
  <button
    type="button"
    class="option"
    class:active={$theme === "light"}
    onclick={() => theme.setTheme("light")}
    aria-label="Light"
    title="Light"
  >
    <Sun size={14} strokeWidth={1.75} />
  </button>
  <button
    type="button"
    class="option"
    class:active={$theme === "dark"}
    onclick={() => theme.setTheme("dark")}
    aria-label="Dark"
    title="Dark"
  >
    <Moon size={14} strokeWidth={1.75} />
  </button>
  <button
    type="button"
    class="option"
    class:active={$theme === "system"}
    onclick={() => theme.setTheme("system")}
    aria-label="System"
    title="System"
  >
    <Monitor size={14} strokeWidth={1.75} />
  </button>
</div>

<style>
  .theme-switcher {
    display: flex;
    align-items: center;
    gap: 2px;
    padding: 2px;
    border-radius: var(--radius-full);
    background: var(--color-bg);
    border: 1px solid var(--color-border);
  }

  .option {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 26px;
    height: 26px;
    border: none;
    border-radius: var(--radius-full);
    background: transparent;
    color: var(--color-text-faint);
    cursor: pointer;
    transition:
      background-color var(--transition-fast),
      color var(--transition-fast);
  }

  .option:hover {
    color: var(--color-text);
  }

  .option.active {
    background: var(--color-surface);
    color: var(--color-accent);
    box-shadow: var(--shadow-xs);
  }
</style>
