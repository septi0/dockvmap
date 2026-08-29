<script lang="ts">
  import { link, push } from "svelte-spa-router";
  import { fly } from "svelte/transition";
  import ChevronDown from "@lucide/svelte/icons/chevron-down";
  import { auth } from "../services/auth";
  import ThemeSwitcher from "./ThemeSwitcher.svelte";

  const reduceMotion =
    typeof matchMedia === "function" &&
    matchMedia("(prefers-reduced-motion: reduce)").matches;
  const menuDuration = reduceMotion ? 0 : 120;

  let open = $state(false);
  let menuRef: HTMLDivElement | undefined = $state();

  let initial = $derived($auth.user?.username?.[0]?.toUpperCase() ?? "?");

  function toggle() {
    open = !open;
  }

  function close() {
    open = false;
  }

  async function handleLogout() {
    close();
    await auth.logout();
    push("/login");
  }

  $effect(() => {
    if (!open) return;

    function handleClick(event: MouseEvent) {
      if (
        menuRef &&
        event.target instanceof Node &&
        !menuRef.contains(event.target)
      ) {
        close();
      }
    }

    function handleKeydown(event: KeyboardEvent) {
      if (event.key === "Escape") close();
    }

    document.addEventListener("click", handleClick);
    document.addEventListener("keydown", handleKeydown);

    return () => {
      document.removeEventListener("click", handleClick);
      document.removeEventListener("keydown", handleKeydown);
    };
  });
</script>

<div class="user-menu" bind:this={menuRef}>
  <button
    class="trigger"
    onclick={toggle}
    aria-haspopup="menu"
    aria-expanded={open}
  >
    <span class="avatar">{initial}</span>
    <span class="username">{$auth.user?.username}</span>
    <span class="chevron"><ChevronDown size={14} strokeWidth={2} /></span>
  </button>

  {#if open}
    <div
      class="menu"
      role="menu"
      transition:fly={{ y: -6, duration: menuDuration }}
    >
      <div class="menu-row">
        <span class="menu-label">Theme</span>
        <ThemeSwitcher />
      </div>

      <div class="menu-divider"></div>

      <a class="menu-item" href="/account/profile" use:link onclick={close}
        >Profile</a
      >
      <button class="menu-item" onclick={handleLogout}>Sign out</button>
    </div>
  {/if}
</div>

<style>
  .user-menu {
    position: relative;
  }

  .trigger {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-2);
    border: none;
    background: transparent;
    border-radius: var(--radius-md);
    cursor: pointer;
    font: inherit;
    color: var(--color-text);
    transition: background-color var(--transition-fast);
  }

  .trigger:hover {
    background: var(--color-surface-hover);
  }

  .username {
    font-size: 0.875rem;
    font-weight: 500;
  }

  .chevron {
    display: inline-flex;
    color: var(--color-text-faint);
    transition: transform var(--transition-fast);
  }

  .trigger[aria-expanded="true"] .chevron {
    transform: rotate(180deg);
  }

  .menu {
    position: absolute;
    top: calc(100% + var(--space-2));
    right: 0;
    min-width: 180px;
    padding: var(--space-2);
    background: var(--color-surface);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-md);
    display: flex;
    flex-direction: column;
    gap: 2px;
    z-index: 10;
  }

  .menu-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-3) var(--space-2);
  }

  .menu-label {
    font-size: 0.8125rem;
    color: var(--color-text-muted);
  }

  .menu-divider {
    height: 1px;
    margin: 0 0 var(--space-1);
    background: var(--color-border);
  }

  .menu-item {
    display: block;
    width: 100%;
    padding: var(--space-2) var(--space-3);
    border: none;
    background: transparent;
    border-radius: var(--radius-sm);
    font: inherit;
    font-size: 0.875rem;
    text-align: left;
    text-decoration: none;
    color: var(--color-text);
    cursor: pointer;
  }

  .menu-item:hover {
    background: var(--color-surface-hover);
  }
</style>
