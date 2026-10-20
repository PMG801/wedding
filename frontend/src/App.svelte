<script lang="ts">
  import { onMount } from 'svelte'
  import EntryNotice from './components/EntryNotice.svelte'
  import GuestHome from './views/GuestHome.svelte'
  import { hasGuestSession } from './lib/gallery/view'
  import { isAdminPath } from './lib/admin/path'
  import { copy } from './lib/i18n/es'

  // /admin is served by the same bundle (Caddy falls back to index.html); the guest flow is untouched elsewhere.
  const adminPage = isAdminPath(location.pathname)

  let access = $state<'checking' | 'granted' | 'missing' | 'unavailable'>('checking')

  onMount(() => {
    if (adminPage) return
    hasGuestSession()
      .then((granted) => (access = granted ? 'granted' : 'missing'))
      .catch(() => (access = 'unavailable'))
  })
</script>

{#if adminPage}
  <!-- Loaded only on /admin, so guests never download the admin code (S11). -->
  {#await import('./views/AdminPage.svelte')}
    <p class="notice" role="status">{copy.gallery.loading}</p>
  {:then { default: AdminPage }}
    <AdminPage />
  {:catch}
    <p class="notice" role="alert">{copy.entry.unavailable}</p>
  {/await}
{:else}
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
{/if}
