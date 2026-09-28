<script lang="ts">
  import { greeting } from './lib/greeting'

  let name = $state('World')
</script>

<main>
  <h1>{greeting(name)}</h1>
  <input bind:value={name} placeholder="Enter a name" />
</main>
