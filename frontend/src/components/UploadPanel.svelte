<script lang="ts">
  import { copy, uploadErrorMessages, uploadStatusLabel } from '../lib/i18n/es'
  import type { UploadItem } from '../lib/upload/queue'

  let {
    items,
    onFiles,
    onRetry,
    onDismissFinished
  }: {
    items: readonly UploadItem[]
    onFiles: (files: File[]) => void
    onRetry: (id: string) => void
    onDismissFinished: () => void
  } = $props()

  const hasFinished = $derived(items.some((item) => item.status === 'done'))

  function select(event: Event) {
    const input = event.currentTarget as HTMLInputElement
    if (input.files !== null && input.files.length > 0) onFiles([...input.files])
    // Allow choosing the same photos again after a failure.
    input.value = ''
  }
</script>

<section class="card upload" aria-labelledby="upload-heading">
  <h2 id="upload-heading" class="visually-hidden">{copy.upload.button}</h2>
  <label class="button primary">
    <input
      class="visually-hidden"
      type="file"
      multiple
      accept="image/jpeg,image/png,image/heic,image/heif,.heic,.heif"
      onchange={select}
    />
    <span aria-hidden="true">＋</span>
    {copy.upload.button}
  </label>
  <p class="hint">{copy.upload.hint}</p>

  {#if items.length > 0}
    <ul class="queue" aria-live="polite">
      {#each items as item (item.id)}
        <li class="queue-item" data-status={item.status}>
          <div class="queue-line">
            <span class="queue-name">{item.name}</span>
            <span class="queue-status">{uploadStatusLabel(item)}</span>
          </div>
          {#if item.status !== 'failed'}
            <progress max="1" value={item.progress} aria-label={item.name}></progress>
          {/if}
          {#if item.status === 'failed' && item.errorCode !== null}
            <p class="queue-error">{uploadErrorMessages[item.errorCode]}</p>
            {#if item.errorCode !== 'unsupported_type' && item.errorCode !== 'too_large'}
              <button class="button small" type="button" onclick={() => onRetry(item.id)}>{copy.upload.retry}</button>
            {/if}
          {:else if item.status === 'done' && item.usedFallback}
            <p class="queue-note">{copy.upload.fallback}</p>
          {/if}
        </li>
      {/each}
    </ul>
    {#if hasFinished}
      <button class="link" type="button" onclick={onDismissFinished}>{copy.upload.clearDone}</button>
    {/if}
  {/if}
</section>

<style>
  .upload {
    display: grid;
    gap: 0.75rem;
    justify-items: center;
    text-align: center;
  }

  .hint {
    margin: 0;
    color: var(--muted);
    font-size: 0.9rem;
  }

  .queue {
    width: 100%;
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 0.5rem;
    text-align: left;
  }

  .queue-item {
    padding: 0.6rem 0.75rem;
    border: 1px solid var(--line);
    border-radius: 0.6rem;
    background: var(--paper);
    display: grid;
    gap: 0.35rem;
  }

  .queue-item[data-status='failed'] {
    border-color: var(--danger-soft);
  }

  .queue-line {
    display: flex;
    justify-content: space-between;
    gap: 0.75rem;
    font-size: 0.9rem;
  }

  .queue-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .queue-status {
    flex-shrink: 0;
    color: var(--muted);
  }

  .queue-item[data-status='done'] .queue-status {
    color: var(--sage-dark);
  }

  progress {
    width: 100%;
    height: 0.4rem;
    accent-color: var(--sage);
  }

  .queue-error,
  .queue-note {
    margin: 0;
    font-size: 0.85rem;
  }

  .queue-error {
    color: var(--danger);
  }

  .queue-note {
    color: var(--muted);
  }

  .queue-item .button {
    justify-self: start;
  }
</style>
