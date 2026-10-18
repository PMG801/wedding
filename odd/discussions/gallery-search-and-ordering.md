# To debate: gallery search and ordering (NOT a task)
Status: DISCUSSION. No implementation authorized.

## Problem
With hundreds of photos and videos the chronological wall becomes chaotic.

## Options to weigh
- **Filters:** all / photos / videos (cheap: `media_type` already stored).
- **Ordering:** recent, oldest, most liked (see `gallery-likes`).
- **Time grouping:** by hour/moment of the day using `confirmed_at` (note it is upload time, not capture time).
- **Uploader attribution or event moments:** needs guest names or albums; conflicts with ADR-0007 (no accounts) and scope creep.
- **Free-text search:** no text data exists on media; only meaningful with captions (new feature).

## Questions
- Which of filter/order solves most of the chaos for the least scope?
- Is capture time (EXIF) worth extracting, given client-side processing (ADR-0004)?
- Do filters compose with the keyset cursor and the likes order?

## Next step
Decide after `gallery-likes` and `admin-moderation` land; then promote to a task file if agreed.
