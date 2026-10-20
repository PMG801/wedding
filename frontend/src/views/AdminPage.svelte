<script lang="ts">
  import AdminHome from './AdminHome.svelte'
  import { adminCopy } from '../lib/i18n/admin'
</script>

<!-- The whole /admin page, header included, lives in its own chunk so guests never download it (S11). -->
<header class="hero">
  <p class="hero-ornament" aria-hidden="true">❦</p>
  <h1>{adminCopy.title}</h1>
</header>

<main>
  <AdminHome />
</main>
