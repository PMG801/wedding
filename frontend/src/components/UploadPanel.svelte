<script lang="ts">
  import { copy, uploadErrorMessage, uploadSummaryLabels } from '../lib/i18n/es'
  import type { UploadItem } from '../lib/upload/queue'
  import { summarizeUploads } from '../lib/upload/summary'

  /** Compact upload progress strip above the bottom navigation (S8). Shown only while uploads exist. */
  let {
    items,
    onRetry,
    onDismissFinished
  }: {
    items: readonly UploadItem[]
    onRetry: (id: string) => void
    onDismissFinished: () => void
  } = $props()

  const summary = $derived(summarizeUploads(items))

  function retryAll() {
    for (const id of summary.retryableIds) onRetry(id)
  }
</script>

{#if summary.state !== 'idle'}
  <section class="strip" data-state={summary.state} aria-label={copy.upload.button}>
    <div class="head">
      <div aria-live="polite" class="text">
        {#if summary.remaining > 0}
          <p class="summary-title">{uploadSummaryLabels.remaining(summary.remaining, summary.total)}</p>
        {:else if summary.state === 'finished'}
          <p class="summary-title">{uploadSummaryLabels.finished(summary.done)}</p>
        {/if}
        {#if summary.retryableIds.length > 0}
          <p class="summary-error">{uploadSummaryLabels.failed(summary.retryableIds.length)}</p>
          {#if summary.failureCode !== null}
            <p class="summary-note">{uploadErrorMessage(summary.failureCode)}</p>
          {/if}
        {/if}
        {#if summary.rejected > 0}
          <p class="summary-note">{uploadSummaryLabels.rejected(summary.rejected)}</p>
        {/if}
      </div>
      <div class="actions">
        {#if summary.retryableIds.length > 0}
          <button class="button small" type="button" onclick={retryAll}>{copy.upload.retry}</button>
        {/if}
        {#if summary.remaining === 0 && summary.done > 0}
          <button class="link" type="button" onclick={onDismissFinished}>{copy.upload.clearDone}</button>
        {/if}
      </div>
    </div>
    {#if summary.total > 0}
      <progress max="1" value={summary.progress} aria-label={copy.upload.progress}></progress>
    {/if}
  </section>
{/if}

<style>
  .strip {
    box-sizing: border-box;
    width: calc(100% - 1.5rem);
    max-width: 928px;
    margin: 0 auto 0.5rem;
    padding: 0.5rem 0.75rem;
    border: 1px solid var(--line);
    border-radius: 0.6rem;
    background: var(--paper);
    box-shadow: 0 2px 8px rgb(47 42 38 / 0.08);
    display: grid;
    gap: 0.35rem;
    /* Notes may add lines, but never cover the whole screen (landscape phones). */
    max-height: 40vh;
    overflow-y: auto;
  }

  .strip[data-state='attention'] {
    border-color: var(--danger-soft);
  }

  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
  }

  .text {
    min-width: 0;
    display: grid;
    gap: 0.1rem;
  }

  .strip p {
    margin: 0;
    font-size: 0.9rem;
    line-height: 1.3;
  }

  .summary-title {
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .strip[data-state='finished'] .summary-title {
    color: var(--sage-dark);
  }

  .summary-error {
    color: var(--danger);
    font-weight: 600;
  }

  .summary-note {
    color: var(--muted);
    font-size: 0.82rem;
  }

  .actions {
    display: flex;
    align-items: center;
    flex-shrink: 0;
    gap: 0.5rem;
  }

  .actions .button.small,
  .actions .link {
    min-height: 2.75rem;
  }

  .actions .link {
    display: inline-flex;
    align-items: center;
  }

  progress {
    width: 100%;
    height: 0.35rem;
    accent-color: var(--sage);
  }
</style>
