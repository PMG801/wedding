<script lang="ts">
  import { onMount } from 'svelte'
  import { AdminSessionRequiredError } from '../lib/api/admin'
  import {
    createPinned,
    deletePinned,
    fetchAdminPinned,
    InvalidPinnedItemError,
    InvalidPinnedOrderError,
    PinnedLimitReachedError,
    PinnedNotFoundError,
    reorderPinned,
    updatePinned,
    type AdminPinnedItem
  } from '../lib/api/pinned'
  import { copy, pinnedDraftErrorMessages } from '../lib/i18n/es'
  import { validatePinnedDraft, type DraftField } from '../lib/pinned/draft'
  import { canAddPinned, moveId } from '../lib/pinned/order'

  let { onSessionLost }: { onSessionLost: () => void } = $props()

  const admin = copy.pinned.admin

  let items = $state<AdminPinnedItem[]>([])
  let loaded = $state(false)
  let busy = $state(false)
  let message = $state('')
  let editingId = $state<string | null>(null)
  let confirmingId = $state<string | null>(null)
  let title = $state('')
  let body = $state('')
  let mediaId = $state('')
  let fieldErrors = $state<Partial<Record<DraftField, string>>>({})

  const creating = $derived(editingId === null)
  const canCreate = $derived(canAddPinned(items.length))

  onMount(() => {
    void load()
  })

  /** Reloads the server list. Never throws: a failed read sets a message or ends the admin session. */
  async function load(): Promise<void> {
    try {
      items = await fetchAdminPinned()
      loaded = true
    } catch (error) {
      if (error instanceof AdminSessionRequiredError) onSessionLost()
      else message = admin.loadFailed
    }
  }

  /** Runs one write. Stale or rejected writes reload the list so the editor matches the server again. */
  async function write(action: () => Promise<void>, failure: string): Promise<boolean> {
    if (busy) return false
    busy = true
    message = ''
    try {
      await action()
      return true
    } catch (error) {
      if (error instanceof AdminSessionRequiredError) onSessionLost()
      else if (error instanceof PinnedLimitReachedError) message = admin.limit
      else if (error instanceof InvalidPinnedItemError) message = admin.invalid
      else if (error instanceof PinnedNotFoundError) {
        message = admin.stale
        await load()
      } else if (error instanceof InvalidPinnedOrderError) {
        message = admin.orderFailed
        await load()
      } else message = failure
      return false
    } finally {
      busy = false
    }
  }

  function resetForm(): void {
    editingId = null
    title = ''
    body = ''
    mediaId = ''
    fieldErrors = {}
  }

  function startEdit(item: AdminPinnedItem): void {
    editingId = item.id
    title = item.title
    body = item.body
    mediaId = item.mediaId ?? ''
    fieldErrors = {}
    confirmingId = null
    message = ''
  }

  async function submit(event: SubmitEvent): Promise<void> {
    event.preventDefault()
    const result = validatePinnedDraft({ title, body, mediaId })
    if (!result.ok) {
      fieldErrors = {}
      for (const failure of result.errors) {
        if (failure.field === 'form') message = pinnedDraftErrorMessages[failure.code]
        else fieldErrors[failure.field] = pinnedDraftErrorMessages[failure.code]
      }
      return
    }
    fieldErrors = {}
    const id = editingId
    const saved = await write(async () => {
      if (id === null) await createPinned(result.draft)
      else await updatePinned(id, result.draft)
      await load()
    }, admin.failed)
    if (saved) resetForm()
  }

  async function move(index: number, delta: -1 | 1): Promise<void> {
    const ids = moveId(
      items.map((item) => item.id),
      index,
      delta
    )
    if (ids === null) return
    await write(async () => {
      items = await reorderPinned(ids)
    }, admin.orderFailed)
  }

  async function remove(item: AdminPinnedItem): Promise<void> {
    const deleted = await write(async () => {
      await deletePinned(item.id)
      await load()
    }, admin.deleteFailed)
    if (deleted) {
      confirmingId = null
      if (editingId === item.id) resetForm()
    }
  }
</script>

<section class="pinned-admin" aria-labelledby="pinned-admin-heading">
  <h3 id="pinned-admin-heading">{admin.heading}</h3>
  <p class="notice">{admin.intro}</p>

  {#if loaded && items.length === 0}
    <p class="notice">{admin.empty}</p>
  {/if}

  <ol class="pinned-list">
    {#each items as item, index (item.id)}
      <li class="entry">
        <div class="entry-text">
          {#if item.image !== null}
            <img src={item.image.thumbUrl} alt={copy.pinned.imageAlt} loading="lazy" decoding="async" />
          {/if}
          <div>
            <strong>{item.title}</strong>
            {#if item.body !== ''}
              <p class="body">{item.body}</p>
            {/if}
            {#if item.mediaId !== null && item.image === null}
              <p class="notice">{admin.hidden}</p>
            {/if}
          </div>
        </div>

        <div class="actions">
          <button class="button small" type="button" onclick={() => move(index, -1)} disabled={busy || index === 0}>
            {admin.moveUp}
          </button>
          <button
            class="button small"
            type="button"
            onclick={() => move(index, 1)}
            disabled={busy || index === items.length - 1}
          >
            {admin.moveDown}
          </button>
          <button class="button small" type="button" onclick={() => startEdit(item)} disabled={busy}>
            {admin.edit}
          </button>
          {#if confirmingId === item.id}
            <span class="confirm">{admin.deleteQuestion}</span>
            <button class="button small" type="button" onclick={() => remove(item)} disabled={busy}>
              {admin.confirmDelete}
            </button>
            <button class="button small" type="button" onclick={() => (confirmingId = null)} disabled={busy}>
              {admin.cancel}
            </button>
          {:else}
            <button class="button small" type="button" onclick={() => (confirmingId = item.id)} disabled={busy}>
              {admin.delete}
            </button>
          {/if}
        </div>
      </li>
    {/each}
  </ol>

  {#if creating && !canCreate}
    <p class="notice" role="status">{admin.limit}</p>
  {/if}

  <form class="pinned-form" onsubmit={submit}>
    <h4>{creating ? admin.createHeading : admin.editHeading}</h4>

    <label for="pinned-title">{admin.titleLabel}</label>
    <input
      id="pinned-title"
      name="title"
      type="text"
      autocomplete="off"
      bind:value={title}
      disabled={busy || (creating && !canCreate)}
      aria-invalid={fieldErrors.title !== undefined}
      aria-describedby={fieldErrors.title !== undefined ? 'pinned-title-error' : undefined}
    />
    {#if fieldErrors.title !== undefined}
      <p class="field-error" id="pinned-title-error">{fieldErrors.title}</p>
    {/if}

    <label for="pinned-body">{admin.bodyLabel}</label>
    <textarea
      id="pinned-body"
      name="body"
      rows="4"
      bind:value={body}
      disabled={busy || (creating && !canCreate)}
      aria-invalid={fieldErrors.body !== undefined}
      aria-describedby={fieldErrors.body !== undefined ? 'pinned-body-error' : undefined}
    ></textarea>
    {#if fieldErrors.body !== undefined}
      <p class="field-error" id="pinned-body-error">{fieldErrors.body}</p>
    {/if}

    <label for="pinned-media">{admin.mediaLabel}</label>
    <input
      id="pinned-media"
      name="media_id"
      type="text"
      autocomplete="off"
      spellcheck="false"
      bind:value={mediaId}
      disabled={busy || (creating && !canCreate)}
      aria-invalid={fieldErrors.mediaId !== undefined}
      aria-describedby={fieldErrors.mediaId !== undefined ? 'pinned-media-error' : 'pinned-media-hint'}
    />
    <p class="notice" id="pinned-media-hint">{admin.mediaHint}</p>
    {#if fieldErrors.mediaId !== undefined}
      <p class="field-error" id="pinned-media-error">{fieldErrors.mediaId}</p>
    {/if}

    <div class="form-actions">
      <button
        class="button primary"
        type="submit"
        disabled={busy || (creating && !canCreate)}
      >
        {creating ? admin.create : admin.save}
      </button>
      {#if !creating}
        <button class="button small" type="button" onclick={resetForm} disabled={busy}>{admin.cancel}</button>
      {/if}
    </div>
  </form>

  {#if message !== ''}
    <p class="admin-error" role="alert">{message}</p>
  {/if}
</section>

<style>
  .pinned-admin {
    display: grid;
    gap: 0.6rem;
  }

  .pinned-admin h3 {
    margin: 0.5rem 0 0;
    font-size: 1.25rem;
  }

  .pinned-admin h4 {
    margin: 0.5rem 0 0;
    font-size: 1.05rem;
  }

  .pinned-list {
    display: grid;
    gap: 0.75rem;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .entry {
    display: grid;
    gap: 0.4rem;
    padding: 0.6rem;
    border: 1px solid var(--line);
    border-radius: 0.6rem;
    background: var(--paper);
  }

  .entry-text {
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 0.6rem;
    align-items: start;
  }

  .entry-text img {
    width: 3.5rem;
    height: 3.5rem;
    object-fit: cover;
    border-radius: 0.4rem;
    background: var(--line);
  }

  .body {
    margin: 0.2rem 0 0;
    white-space: pre-line;
    overflow-wrap: anywhere;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.4rem;
  }

  .confirm {
    color: var(--danger);
    font-size: 0.9rem;
  }

  .pinned-form {
    display: grid;
    gap: 0.4rem;
  }

  .pinned-form label {
    color: var(--muted);
  }

  .pinned-form input,
  .pinned-form textarea {
    padding: 0.5rem 0.75rem;
    border: 1px solid var(--line);
    border-radius: 0.6rem;
    background: var(--paper);
    color: var(--ink);
    font: inherit;
  }

  .pinned-form input {
    min-height: 2.5rem;
  }

  .form-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
  }

  .notice {
    margin: 0;
    color: var(--muted);
  }

  .field-error,
  .admin-error {
    margin: 0;
    color: var(--danger);
  }
</style>
