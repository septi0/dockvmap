<script lang="ts">
  import type { Snippet } from 'svelte'

  let {
    variant = 'primary',
    size = 'md',
    type = 'button',
    disabled = false,
    onclick,
    children,
  }: {
    variant?: 'primary' | 'secondary' | 'danger' | 'warning'
    size?: 'md' | 'sm'
    type?: 'button' | 'submit'
    disabled?: boolean
    onclick?: () => void
    children: Snippet
  } = $props()
</script>

<button class="btn btn-{variant}" class:btn-sm={size === 'sm'} {type} {disabled} {onclick}>
  {@render children()}
</button>
