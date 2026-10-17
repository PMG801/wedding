# Guest video upload and playback
Branch: `feat/video-upload` from `develop`; delivery: review slices toward `develop`.
Tests: `cd backend && go test ./... && go vet ./...`; `cd frontend && npm test && npm run build`; `scripts/smoke.sh`.

## Objective
Let guests upload videos (up to `BODA_MAX_VIDEO_BYTES`, 1.5 GB) from the QR flow, see them in the gallery with a poster, and play or download them.

## Accepted scope and constraints
- Follow ADR-0002, 0004 (client poster, no server transcoding), 0005 (streaming idempotent upload; one video at a time on the client; TUS only if runbook T2 fails), 0006/0011 (media behind the guest session, Range support).
- Reuse the photo pipeline: same `PUT /api/media/{id}`, same derivatives route for the poster (`t`/`d` JPEG), same gallery, same reconciliation.
- Formats: MP4/M4V and QuickTime MOV (ISO base media `ftyp`), detected by signature. No WebM, no server transcoding, no moderation.
- Admin moderation, security hardening and `boda reindex` are out of scope (user decision, L2).
- Local work-unit commits authorized under ODD; push, PR and merge remain the user's decisions.
- Engram mirror topic: `odd/video-upload/tasks`.

## Specs
- **S1 — Limits.** Photos keep `BODA_MAX_PHOTO_BYTES`; videos use `BODA_MAX_VIDEO_BYTES`. The limit is chosen by detected type; declared size above the video limit is rejected before reading the body.
- **S2 — Detection.** `ftyp` brands for HEIC/HEIF stay photos; brands `isom`, `iso2`, `mp41`, `mp42`, `avc1`, `M4V `, `qt  ` are videos (`video/mp4`, `video/quicktime`). Anything else stays `unsupported_photo_format`.
- **S3 — Poster.** The browser captures one frame from an off-screen `<video>` and sends it as `t` and `d` JPEG derivatives; if it fails, upload the original only and show a placeholder tile.
- **S4 — Client queue.** A video uploads alone (ADR-0005: concurrency 1 for video); photos keep two at a time. Same retry/backoff and idempotent ID.
- **S5 — Playback.** Detail dialog plays with `<video controls>`; if the browser cannot play it, offer the download. UI copy in Spanish, code in English.
- **S6 — Docs.** `docs/api.md`, `docs/runbook.md` (Spanish) describe video limits, formats and checks.

## Tasks
- [x] **T1 [S1, S2] — Backend: detect and store videos**
  - Route: inline parent, test-first.
  - Scope: `DetectMediaType` (video brands), type-based size limit in `Store`, gallery MIME passthrough.
  - Files: `backend/internal/upload/**`.
  - Risk: item 6 covered by tests; shared photo path touched, so photo behavior tests must stay green.
  - Evidence: RED `go test ./internal/upload` failed to build (`DetectMediaType`, `Options.MaxVideoBytes`, `ErrPhotoTooLarge` undefined). GREEN: `cd backend && go test ./... && go vet ./...` and `gofmt -l` clean, all existing photo tests unchanged and passing. Tests cover MP4/M4V/MOV brands, HEIC/HEIF still photos, unknown brand rejected, `DetectPhotoType` still rejecting videos, video stored with `.mp4` and byte-identical, `ErrMediaTooLarge` before reading, `ErrPhotoTooLarge`/`ErrVideoTooLarge` after detection, and no leftover temp/original files. Gallery already returns the stored `media_type`, so no gallery change was needed; `cmd/boda` wiring of `MaxVideoBytes` moves to T2.
  - Commit: see git log (`feat(upload): detect and store MP4/MOV videos`).
- [x] **T2 [S1] — HTTP: video limit, errors and contract**
  - Scope: handler accepts up to the video limit, new `video_too_large` response, API docs.
  - Evidence: RED `go test ./internal/httpapi` failed to build (`WithMaxVideoBytes` undefined). GREEN: `cd backend && go test ./... && go vet ./...` pass; `NewHandler` gained only a variadic option, so existing callers and photo tests are unchanged. Tests: video 201 then idempotent 200, photo above photo limit 413 `photo_too_large`, oversize 413 `media_too_large`, no stray originals/temp files, derivatives keep the photo limit. `docs/api.md` updated. Commit: `feat(upload): accept videos on the media upload route`.
  - Files: `backend/internal/httpapi/**`, `backend/cmd/boda/**` (pass `MaxVideoBytes` to the store and handler), `docs/api.md`. Early 413 keeps `photo_too_large` when the video limit is not above the photo limit (existing contract); otherwise `media_too_large`; post-detection `photo_too_large`/`video_too_large`.
- [x] **T3 [S5] — Caddy and smoke: serve video with Range**
  - Scope: confirm `video/mp4`/`video/quicktime` types, extend `scripts/smoke.sh` with an MP4 upload, Range 206 and download.
  - Files: `scripts/smoke.sh` (Caddyfile needed no change).
  - Evidence: `scripts/smoke.sh` exit 0 with the new section: MP4 upload 201 and idempotent retry 200, gallery lists `.mp4` with mime `video/mp4`, Caddy serves `Content-Type: video/mp4` byte-identical, `Range: bytes=100-1099` returns 206 with `Content-Range: bytes 100-1099/*`. Caddy's built-in MIME table covers `.mp4`/`.mov`, so no header rule was added. MOV type was not exercised by smoke (same table). Commit: `test(smoke): cover MP4 upload, serving and Range`.
  - Risk: item 5 (proxy config).
- [x] **T4 [S3, S4] — Frontend: poster capture and video queue**
  - Scope: `lib/media/poster.ts`, accepted types/size by kind, video-alone concurrency, Spanish errors.
  - Files: `frontend/src/lib/**`, `frontend/src/views/GuestHome.svelte`, `frontend/src/components/UploadPanel.svelte`.
  - Evidence: RED `npx vitest run src/lib` failed 2 tests (videos rejected as unsupported; no video-alone scheduling). GREEN: 55/55 tests, `svelte-check` 0 errors, `npm run build` OK. Tests: MP4/MOV accepted by type or extension, video limit separate from photo limit, strict FIFO with a video running alone (waits for running photos, holds back later ones), poster codec dispatch and null fallback. The previous test asserting `.mov` is unsupported was updated to `.avi` because accepting MOV is the requested change. The `<video>` frame codec (`lib/media/poster.ts`) and `accept` attribute are browser-only: exercised in the T6 manual device checks. Spanish copy now mentions videos and 1.5 GB.
  - Commit: `feat(frontend): upload videos with a poster frame`.
- [x] **T5 [S5] — Frontend: video tile and player**
  - Scope: play badge on tiles, `<video>` in `PhotoDialog`, download fallback, no HEIC-style `<img>` paint of videos.
  - Files: `frontend/src/lib/gallery/**`, `frontend/src/lib/i18n/es.ts`, `frontend/src/components/GalleryGrid.svelte`, `frontend/src/components/PhotoDialog.svelte`.
  - Evidence: RED observed with the old `view.ts` (`isVideoItem is not a function`); GREEN: 56/56 frontend tests, `svelte-check` 0 errors, `npm run build` OK. Videos never use the original in `<img>`; the grid shows the poster thumbnail (or the placeholder) with a play badge; the dialog plays `<video controls playsinline>` with the poster, and on a playback error shows the Spanish message while the download button stays. Real playback, swipe and HEVC fallback are browser-only and covered by the manual T6 checks.
  - Commit: `feat(frontend): play videos in the gallery`.
- [x] **T6 [S6] — Docs and device checks**
  - Scope: runbook updates; record that runbook T1/T2 (real devices, 4G) remain manual.
  - Files: `docs/runbook.md`, `docs/api.md` (updated in T2).
  - Evidence: final checks all pass: `cd backend && go test ./... && go vet ./...` and `gofmt -l` clean; `cd frontend && npm test` 56/56, `npm run build` OK, `svelte-check` 0 errors; `scripts/smoke.sh` exit 0 including the video section; `git diff --check` clean. Behavior diff on shared code: photo uploads keep their codes (`photo_too_large` 413, `unsupported_photo_format` 415); the one intended change is that with the default config an oversized non-media file between 50 MB and 1.5 GB is now rejected with 415 after reading its first 64 bytes instead of 413 before reading. Not exercised: real devices, HEVC fallback in browsers, 4G (runbook T1/T2), and MOV through Caddy.
  - Commit: `docs(runbook): document video uploads`.

## Acceptance criteria
1. An MP4/MOV up to the video limit uploads, is confirmed once, and retries do not rewrite it.
2. Oversized videos, photos above the photo limit, and unknown signatures are rejected with stable error codes without leaving stored data.
3. The gallery shows a video poster (or placeholder), plays it with Range seeking, and offers a byte-identical download.
4. Backend tests/vet, frontend tests/build and the smoke test pass; limitations (no device or 4G test) are recorded.

## Next step
All tasks T1-T6 are committed locally. Pending: manual device and 4G checks (runbook T1/T2), then push/PR decisions (two slices: T1-T3 backend, T4-T6 frontend and docs). Risk: item 3 touched shared upload route and limits (contract kept, one noted difference above).

Slice boundary: slice 1 = T1-T3 (backend and smoke, `docs(odd)` plan through `test(smoke)`); slice 2 = T4-T6 (frontend and docs).

## Log
- L1 — 2026-10-17, user (verbatim): “Por lo que el paso a proceder va a ser el implementar la subida de video”
- L2 — 2026-10-17, user: administration is postponed until much later; priority is feasibility and a usable product, security afterwards.
