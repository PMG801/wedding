<script lang="ts">
  import { onMount } from 'svelte'
  import { fetchHealth } from './lib/api/health'

  let status = $state<'loading' | 'ok' | 'unavailable'>('loading')

  onMount(() => {
    let active = true

    fetchHealth()
      .then(() => {
        if (active) status = 'ok'
      })
      .catch(() => {
        if (active) status = 'unavailable'
      })

    return () => {
      active = false
    }
  })
</script>

<main>
  <h1>Wedding memories</h1>
  <p>API: {status === 'loading' ? 'loading' : status === 'ok' ? 'ok' : 'unavailable'}</p>
</main>
