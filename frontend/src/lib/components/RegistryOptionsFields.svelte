<script lang="ts">
  let {
    insecure = $bindable(false),
    allowSelfSignedCerts = $bindable(false),
  }: {
    insecure?: boolean;
    allowSelfSignedCerts?: boolean;
  } = $props();
</script>

<label class="checkbox">
  <input type="checkbox" bind:checked={insecure} />
  <span>Allow insecure (HTTP) connections</span>
</label>

<label class="checkbox">
  <input type="checkbox" bind:checked={allowSelfSignedCerts} />
  <span>Allow self-signed certificates</span>
</label>
