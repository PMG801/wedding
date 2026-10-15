<script lang="ts">
  import { onMount } from 'svelte'
  import EntryNotice from './components/EntryNotice.svelte'
  import GuestHome from './views/GuestHome.svelte'
  import { hasGuestSession } from './lib/gallery/view'
  import { copy } from './lib/i18n/es'

  let access = $state<'checking' | 'granted' | 'missing' | 'unavailable'>('checking')

  onMount(() => {
    hasGuestSession()
      .then((granted) => (access = granted ? 'granted' : 'missing'))
      .catch(() => (access = 'unavailable'))
  })
</script>

<header class="hero">
  <p class="hero-ornament" aria-hidden="true">❦</p>
  <h1>{copy.title}</h1>
  <p class="hero-subtitle">{copy.subtitle}</p>
</header>

<main>
  {#if access === 'checking'}
    <p class="notice" role="status">{copy.entry.checking}</p>
  {:else if access === 'granted'}
    <GuestHome onSessionLost={() => (access = 'missing')} />
  {:else}
    <EntryNotice unavailable={access === 'unavailable'} />
  {/if}
</main>
