<script lang="ts">
  import { onMount } from 'svelte'
  import {
    AdminSessionRequiredError,
    fetchAdminStatus,
    loginAdmin,
    logoutAdmin,
    setUploadsEnabled,
    type AdminStatus
  } from '../lib/api/admin'
  import { canSubmitPassword, diskNotice, formatGigabytes, uploadsToggle } from '../lib/admin/view'
  import { adminCopy } from '../lib/i18n/admin'

  type Phase = 'checking' | 'login' | 'ready' | 'unavailable'

  let phase = $state<Phase>('checking')
  let status = $state<AdminStatus | null>(null)
  let password = $state('')
  let busy = $state(false)
  let message = $state('')

  const toggle = $derived(status === null ? null : uploadsToggle(status))
  const notice = $derived(status === null ? null : diskNotice(status))

  onMount(() => {
    void refresh()
  })

  async function refresh(): Promise<void> {
    try {
      status = await fetchAdminStatus()
      phase = 'ready'
    } catch (error) {
      status = null
      phase = error instanceof AdminSessionRequiredError ? 'login' : 'unavailable'
    }
  }

  async function submitLogin(event: SubmitEvent): Promise<void> {
    event.preventDefault()
    if (busy || !canSubmitPassword(password)) return
    busy = true
    message = ''
    try {
      const result = await loginAdmin(password)
      password = ''
      if (result === 'invalid_credentials') {
        message = adminCopy.login.invalid
        return
      }
      await refresh()
    } catch {
      message = adminCopy.login.failed
    } finally {
      busy = false
    }
  }

  async function changeUploads(): Promise<void> {
    if (busy || toggle === null) return
    busy = true
    message = ''
    try {
      status = await setUploadsEnabled(toggle.nextEnabled)
    } catch (error) {
      if (error instanceof AdminSessionRequiredError) {
        status = null
        phase = 'login'
      } else {
        message = adminCopy.error
      }
    } finally {
      busy = false
    }
  }

  async function logout(): Promise<void> {
    if (busy) return
    busy = true
    message = ''
    try {
      await logoutAdmin()
      status = null
      password = ''
      phase = 'login'
    } catch {
      message = adminCopy.error
    } finally {
      busy = false
    }
  }
</script>

<section class="card admin">
  {#if phase === 'checking'}
    <p class="notice" role="status">{adminCopy.loading}</p>
  {:else if phase === 'unavailable'}
    <p role="alert">{adminCopy.unavailable}</p>
  {:else if phase === 'login'}
    <h2>{adminCopy.login.heading}</h2>
    <form class="admin-form" onsubmit={submitLogin}>
      <label for="admin-password">{adminCopy.login.password}</label>
      <input
        id="admin-password"
        name="password"
        type="password"
        autocomplete="current-password"
        bind:value={password}
        disabled={busy}
        required
      />
      <button class="button primary" type="submit" disabled={busy || !canSubmitPassword(password)}>
        {busy ? adminCopy.login.submitting : adminCopy.login.submit}
      </button>
    </form>
  {:else if status !== null && toggle !== null}
    <h2>{adminCopy.uploads.heading}</h2>
    <p class="admin-state" aria-live="polite">{toggle.stateLabel}</p>
    <button class="button primary" type="button" onclick={changeUploads} disabled={busy}>
      {toggle.actionLabel}
    </button>

    {#if notice !== null}
      <p class="admin-notice" role="status">{notice}</p>
    {/if}

    <h2>{adminCopy.disk.heading}</h2>
    <dl class="admin-disk">
      <dt>{adminCopy.disk.free}</dt>
      <dd>{formatGigabytes(status.freeBytes)}</dd>
      <dt>{adminCopy.disk.reserved}</dt>
      <dd>{formatGigabytes(status.reservedBytes)}</dd>
    </dl>

    <button class="button small" type="button" onclick={logout} disabled={busy}>{adminCopy.logout}</button>
  {/if}

  {#if message !== ''}
    <p class="admin-error" role="alert">{message}</p>
  {/if}
</section>

<style>
  .admin {
    display: grid;
    gap: 0.75rem;
    max-width: 28rem;
    margin: 0 auto;
  }

  .admin h2 {
    margin: 0.5rem 0 0;
    font-size: 1.5rem;
  }

  .admin-form {
    display: grid;
    gap: 0.75rem;
  }

  .admin-form label {
    color: var(--muted);
  }

  .admin-form input {
    min-height: 2.75rem;
    padding: 0.5rem 0.75rem;
    border: 1px solid var(--line);
    border-radius: 0.6rem;
    background: var(--paper);
    color: var(--ink);
    font: inherit;
  }

  .admin-state {
    margin: 0;
    font-weight: 600;
  }

  .admin-notice {
    margin: 0;
    color: var(--danger);
  }

  .admin-error {
    margin: 0;
    color: var(--danger);
  }

  .admin-disk {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 0.4rem 1rem;
    margin: 0;
  }

  .admin-disk dt {
    color: var(--muted);
  }

  .admin-disk dd {
    margin: 0;
    font-weight: 600;
    text-align: right;
  }
</style>
