<script lang="ts">
  import { copy } from '../lib/i18n/es'

  let { unavailable }: { unavailable: boolean } = $props()
</script>

<section class="card entry">
  {#if unavailable}
    <p role="alert">{copy.entry.unavailable}</p>
  {:else}
    <h2>{copy.entry.heading}</h2>
    <p>{copy.entry.body}</p>
  {/if}
</section>

<style>
  .entry {
    text-align: center;
  }
</style>
