CREATE TABLE media (
    id TEXT PRIMARY KEY NOT NULL,
    storage_name TEXT NOT NULL UNIQUE,
    media_type TEXT NOT NULL,
    size_bytes INTEGER NOT NULL CHECK (typeof(size_bytes) = 'integer' AND size_bytes > 0),
    confirmed_at INTEGER NOT NULL CHECK (typeof(confirmed_at) = 'integer'),
    visibility TEXT NOT NULL DEFAULT 'visible' CHECK (visibility IN ('visible', 'hidden'))
);

CREATE INDEX media_visible_cursor
    ON media (confirmed_at DESC, id DESC)
    WHERE visibility = 'visible';

CREATE INDEX media_hidden_cursor
    ON media (confirmed_at DESC, id DESC)
    WHERE visibility = 'hidden';

CREATE TABLE settings (
    key TEXT PRIMARY KEY NOT NULL,
    value TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);
