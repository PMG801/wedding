<script lang="ts">
  import type { GalleryView } from '../lib/gallery/feed'
  import { copy } from '../lib/i18n/es'

  /**
   * Fixed bottom navigation (S8): the two gallery views and the upload control.
   * The upload control is a label around the only file input of the guest page.
   */
  let {
    view,
    onViewChange,
    onFiles
  }: {
    view: GalleryView
    /** Called with the tapped view; the parent decides whether it is a switch or a scroll to top. */
    onViewChange: (view: GalleryView) => void
    onFiles: (files: File[]) => void
  } = $props()

  function select(event: Event) {
    const input = event.currentTarget as HTMLInputElement
    if (input.files !== null && input.files.length > 0) onFiles([...input.files])
    // Allow choosing the same photos again after a failure.
    input.value = ''
  }
</script>

<nav class="nav" aria-label={copy.gallery.viewsLabel}>
  <div class="row">
    <!-- Each view tab is a toggle button (aria-pressed) inside the labelled navigation landmark. -->
    <button class="tab" type="button" aria-pressed={view === 'recent'} onclick={() => onViewChange('recent')}>
      <svg class="icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
        <rect x="3" y="3" width="7" height="7" rx="1.5" />
        <rect x="14" y="3" width="7" height="7" rx="1.5" />
        <rect x="3" y="14" width="7" height="7" rx="1.5" />
        <rect x="14" y="14" width="7" height="7" rx="1.5" />
      </svg>
      <span class="label">{copy.gallery.views.recent}</span>
    </button>

    <label class="upload">
      <input
        class="visually-hidden"
        type="file"
        multiple
        accept="image/jpeg,image/png,image/heic,image/heif,video/mp4,video/quicktime,.heic,.heif,.mp4,.m4v,.mov"
        aria-label={copy.upload.button}
        onchange={select}
      />
      <span class="plus" aria-hidden="true">
        <svg viewBox="0 0 24 24" focusable="false">
          <path d="M12 5v14M5 12h14" />
        </svg>
      </span>
      <span class="label">{copy.upload.navLabel}</span>
    </label>

    <button class="tab" type="button" aria-pressed={view === 'top'} onclick={() => onViewChange('top')}>
      <svg class="icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
        <path d="M12 20.5s-7.5-4.6-7.5-10A4.2 4.2 0 0 1 12 7.8a4.2 4.2 0 0 1 7.5 2.7c0 5.4-7.5 10-7.5 10z" />
      </svg>
      <span class="label">{copy.gallery.views.top}</span>
    </button>
  </div>
</nav>

<style>
  .nav {
    box-sizing: border-box;
    height: 3.5rem;
    background: var(--paper);
    border-top: 1px solid var(--line);
    box-shadow: 0 -2px 10px rgb(47 42 38 / 0.06);
  }

  .row {
    box-sizing: border-box;
    max-width: 960px;
    height: 100%;
    margin: 0 auto;
    padding: 0 1rem;
    display: grid;
    grid-template-columns: 1fr auto 1fr;
    align-items: center;
    gap: 0.5rem;
  }

  .tab,
  .upload {
    position: relative;
    box-sizing: border-box;
    min-height: 44px;
    min-width: 44px;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    /* Icon, circle and label share one top-aligned column, so the three labels line up. */
    justify-content: flex-start;
    gap: 0.15rem;
    padding: 0.3rem 0.5rem 0;
    border: none;
    background: none;
    color: var(--muted);
    font: inherit;
    font-size: 0.72rem;
    line-height: 1.1;
    cursor: pointer;
  }

  .tab .icon {
    margin: 0.5rem 0;
  }

  .tab .icon,
  .plus svg {
    width: 1.25rem;
    height: 1.25rem;
    fill: none;
    stroke: currentColor;
    stroke-width: 1.8;
    stroke-linecap: round;
    stroke-linejoin: round;
  }

  /* Static active marker; the animated indicator comes with T4. */
  .tab[aria-pressed='true'] {
    color: var(--sage-dark);
    font-weight: 600;
  }

  .tab[aria-pressed='true']::before {
    content: '';
    position: absolute;
    top: 0;
    left: 30%;
    right: 30%;
    height: 2px;
    border-radius: 0 0 2px 2px;
    background: var(--sage-dark);
  }

  .plus {
    display: grid;
    place-items: center;
    width: 2.25rem;
    height: 2.25rem;
    border-radius: 50%;
    background: var(--sage);
    color: #fff;
    box-shadow: 0 2px 6px rgb(86 108 75 / 0.35);
  }

  .plus svg {
    width: 1.2rem;
    height: 1.2rem;
    stroke-width: 2.2;
  }

  .upload {
    color: var(--sage-dark);
    font-weight: 600;
  }

  .tab:focus-visible,
  .upload:has(input:focus-visible) {
    outline: 3px solid var(--sage-dark);
    outline-offset: -3px;
    border-radius: 0.5rem;
  }

  .label {
    white-space: nowrap;
  }
</style>
