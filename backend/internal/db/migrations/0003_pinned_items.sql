-- Pinned text entries written by the administrator (ADR-0013). The media table
-- is untouched; media_id is an optional reference to an existing medium.
-- The application keeps position values from 0 to 4 and rewrites them in one
-- transaction; UNIQUE only guarantees that no two entries share a position.
CREATE TABLE pinned_items (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL CHECK (length(title) BETWEEN 1 AND 120),
    body TEXT NOT NULL DEFAULT '' CHECK (length(body) <= 2000),
    media_id TEXT NULL REFERENCES media (id),
    position INTEGER NOT NULL UNIQUE CHECK (position >= 0),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
