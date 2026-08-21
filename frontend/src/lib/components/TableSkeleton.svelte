<script lang="ts">
  let {
    columns,
    rows = 5,
  }: {
    columns: number;
    rows?: number;
  } = $props();

  const widths = [90, 60, 75, 45, 85, 55, 70];
</script>

<div class="card">
  <table class="table">
    <thead>
      <tr>
        {#each Array.from({ length: columns }) as _, colIndex (colIndex)}
          <th
            ><span class="bar" style:width="{widths[colIndex % widths.length]}%"
            ></span></th
          >
        {/each}
      </tr>
    </thead>
    <tbody>
      {#each Array.from({ length: rows }) as _, rowIndex (rowIndex)}
        <tr>
          {#each Array.from({ length: columns }) as _, colIndex (colIndex)}
            <td>
              <span
                class="bar"
                style:width="{widths[(colIndex + rowIndex) % widths.length]}%"
              ></span>
            </td>
          {/each}
        </tr>
      {/each}
    </tbody>
  </table>
</div>

<style>
  .bar {
    display: inline-block;
    height: 12px;
    border-radius: var(--radius-sm);
    background: linear-gradient(
      90deg,
      var(--color-border) 25%,
      var(--color-surface-hover) 50%,
      var(--color-border) 75%
    );
    background-size: 200% 100%;
    animation: shimmer 1.5s ease-in-out infinite;
  }

  @keyframes shimmer {
    0% {
      background-position: 200% 0;
    }
    100% {
      background-position: -200% 0;
    }
  }
</style>
