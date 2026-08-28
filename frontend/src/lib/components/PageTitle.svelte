<script lang="ts">
  let { title }: { title?: string } = $props();
</script>

<svelte:head>
  <title>{title ? `${title} · DockVMap` : "DockVMap"}</title>
</svelte:head>
