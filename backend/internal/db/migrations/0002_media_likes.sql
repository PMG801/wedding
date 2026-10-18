-- Anonymous «me gusta» per guest identity (ADR-0012). like_count is the
-- aggregate kept in the same transaction as media_likes rows.
ALTER TABLE media ADD COLUMN like_count INTEGER NOT NULL DEFAULT 0
    CHECK (typeof(like_count) = 'integer' AND like_count >= 0);

CREATE TABLE media_likes (
    media_id TEXT NOT NULL REFERENCES media (id),
    guest_id TEXT NOT NULL
        CHECK (length(guest_id) = 32 AND guest_id NOT GLOB '*[^0-9a-f]*'),
    created_at INTEGER NOT NULL CHECK (typeof(created_at) = 'integer'),
    PRIMARY KEY (media_id, guest_id)
) WITHOUT ROWID;

-- Ranking of the 50 most liked visible media; only rows with at least one like.
CREATE INDEX media_visible_popularity
    ON media (like_count DESC, confirmed_at DESC, id DESC)
    WHERE visibility = 'visible' AND like_count > 0;
