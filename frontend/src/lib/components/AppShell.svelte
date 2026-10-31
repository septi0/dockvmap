<script lang="ts">
  import { onMount } from "svelte";
  import type { Snippet } from "svelte";
  import { link } from "svelte-spa-router";
  import Menu from "@lucide/svelte/icons/menu";
  import active from "svelte-spa-router/active";
  import Logo from "./Logo.svelte";
  import UserMenu from "./UserMenu.svelte";
  import UpdatesBadge from "./UpdatesBadge.svelte";
  import { getVersion } from "../api/system";

  let { children }: { children: Snippet } = $props();

  let version = $state("");
  let menuOpen = $state(false);

  function closeMenuOnEscape(event: KeyboardEvent) {
    if (event.key === "Escape") menuOpen = false;
  }

  onMount(async () => {
    try {
      version = (await getVersion()).version;
    } catch {
      // non-essential chrome, ignore failures
    }
  });

  const navItems = [
    { label: "Dashboard", href: "/" },
    { label: "Registries", href: "/registries" },
    { label: "Virtual Images", href: "/images" },
    { label: "Groups", href: "/groups" },
    { label: "Proxy Tokens", href: "/proxy-tokens" },
    { label: "Tag activity", href: "/tag-activity" },
    { label: "System", href: "/system" },
  ];
</script>

<svelte:window onkeydown={menuOpen ? closeMenuOnEscape : undefined} />

<div class="shell">
  {#if menuOpen}
    <button
      type="button"
      class="backdrop"
      aria-label="Close menu"
      onclick={() => (menuOpen = false)}
    ></button>
  {/if}

  <aside class="sidebar" class:open={menuOpen}>
    <div class="brand"><Logo size="sm" /></div>

    <nav>
      <ul>
        {#each navItems as item}
          <li>
            <a
              href={item.href}
              use:link
              use:active
              onclick={() => (menuOpen = false)}>{item.label}</a
            >
          </li>
        {/each}
      </ul>
    </nav>

    <p class="sidebar-footer muted">
      DockVMap{version ? ` ${version}` : ""} · AGPL-3.0 ·
      <a href="https://github.com/septi0/dockvmap" target="_blank" rel="noopener noreferrer">Source</a>
    </p>
  </aside>

  <div class="main">
    <header class="topbar">
      <button
        type="button"
        class="icon-button menu-button"
        aria-label="Open menu"
        aria-expanded={menuOpen}
        onclick={() => (menuOpen = true)}
      >
        <Menu size={22} strokeWidth={1.75} />
      </button>
      <UpdatesBadge />
      <UserMenu />
    </header>

    <main class="content">
      {@render children()}
    </main>
  </div>
</div>

<style>
  .shell {
    display: flex;
    min-height: 100vh;
    min-height: 100dvh;
    max-width: 1920px;
    margin: 0 auto;
  }

  .sidebar {
    display: flex;
    flex-direction: column;
    width: 232px;
    flex-shrink: 0;
    background: var(--color-surface);
    border-right: 1px solid var(--color-border);
    padding: var(--space-5) var(--space-4);
  }

  .sidebar-footer {
    margin: auto 0 0;
    padding: 0 var(--space-2);
    font-size: 0.75rem;
  }

  .sidebar-footer a {
    color: inherit;
    text-decoration: underline;
  }

  .sidebar-footer a:hover {
    color: var(--color-text);
  }

  .brand {
    margin-bottom: var(--space-6);
    padding: 0 var(--space-2);
  }

  nav ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  nav a {
    display: flex;
    align-items: center;
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    font-size: 0.875rem;
    font-weight: 500;
    text-decoration: none;
    color: var(--color-text-muted);
    transition:
      background-color var(--transition-fast),
      color var(--transition-fast);
  }

  nav a:hover {
    background: var(--color-surface-hover);
    color: var(--color-text);
  }

  nav a:global(.active) {
    background: var(--color-accent-muted-bg);
    color: var(--color-accent);
  }

  .main {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .topbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-6);
    background: var(--color-surface);
    border-bottom: 1px solid var(--color-border);
  }

  .content {
    flex: 1;
    padding: var(--space-8);
  }

  .menu-button,
  .backdrop {
    display: none;
  }

  @media (max-width: 768px) {
    .sidebar {
      position: fixed;
      inset: 0 auto 0 0;
      z-index: 90;
      visibility: hidden;
      transform: translateX(-100%);
      transition:
        transform var(--transition),
        visibility var(--transition);
    }

    .sidebar.open {
      visibility: visible;
      transform: none;
      box-shadow: var(--shadow-md);
    }

    .backdrop {
      display: block;
      position: fixed;
      inset: 0;
      z-index: 80;
      padding: 0;
      border: none;
      background: rgba(0, 0, 0, 0.5);
    }

    .menu-button {
      display: inline-flex;
      margin-right: auto;
    }

    .topbar {
      padding: var(--space-2) var(--space-4);
    }

    .content {
      padding: var(--space-4);
    }
  }
</style>
