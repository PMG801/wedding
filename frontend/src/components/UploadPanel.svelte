<script lang="ts">
  import { copy, uploadErrorMessages, uploadSummaryLabels } from '../lib/i18n/es'
  import type { UploadItem } from '../lib/upload/queue'
  import { summarizeUploads } from '../lib/upload/summary'

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

  const summary = $derived(summarizeUploads(items))

  function retryAll() {
    for (const id of summary.retryableIds) onRetry(id)
  }

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
      accept="image/jpeg,image/png,image/heic,image/heif,video/mp4,video/quicktime,.heic,.heif,.mp4,.m4v,.mov"
      onchange={select}
    />
    <span aria-hidden="true">＋</span>
    {copy.upload.button}
  </label>
  <p class="hint">{copy.upload.hint}</p>

  {#if summary.state !== 'idle'}
    <div class="summary" data-state={summary.state}>
      <div aria-live="polite">
        {#if summary.remaining > 0}
          <p class="summary-title">{uploadSummaryLabels.remaining(summary.remaining, summary.total)}</p>
        {:else if summary.state === 'finished'}
          <p class="summary-title">{uploadSummaryLabels.finished(summary.done)}</p>
        {/if}
        {#if summary.retryableIds.length > 0}
          <p class="summary-error">{uploadSummaryLabels.failed(summary.retryableIds.length)}</p>
          {#if summary.failureCode !== null}
            <p class="summary-note">{uploadErrorMessages[summary.failureCode]}</p>
          {/if}
        {/if}
        {#if summary.rejected > 0}
          <p class="summary-note">{uploadSummaryLabels.rejected(summary.rejected)}</p>
        {/if}
      </div>
      {#if summary.total > 0}
        <progress max="1" value={summary.progress} aria-label={copy.upload.progress}></progress>
      {/if}
      {#if summary.retryableIds.length > 0}
        <button class="button small" type="button" onclick={retryAll}>{copy.upload.retry}</button>
      {/if}
      {#if summary.remaining === 0 && summary.done > 0}
        <button class="link" type="button" onclick={onDismissFinished}>{copy.upload.clearDone}</button>
      {/if}
    </div>
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

  .summary {
    width: 100%;
    padding: 0.75rem;
    border: 1px solid var(--line);
    border-radius: 0.6rem;
    background: var(--paper);
    display: grid;
    gap: 0.5rem;
    justify-items: center;
  }

  .summary[data-state='attention'] {
    border-color: var(--danger-soft);
  }

  .summary p {
    margin: 0;
  }

  .summary-title {
    font-weight: 600;
  }

  .summary[data-state='finished'] .summary-title {
    color: var(--sage-dark);
  }

  .summary-error {
    color: var(--danger);
    font-weight: 600;
  }

  .summary-note {
    color: var(--muted);
    font-size: 0.85rem;
  }

  progress {
    width: 100%;
    height: 0.5rem;
    accent-color: var(--sage);
  }
</style>
