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
            ><span
              class="skeleton-bar"
              style:width="{widths[colIndex % widths.length]}%"
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
                class="skeleton-bar"
                style:width="{widths[(colIndex + rowIndex) % widths.length]}%"
              ></span>
            </td>
          {/each}
        </tr>
      {/each}
    </tbody>
  </table>
</div>
