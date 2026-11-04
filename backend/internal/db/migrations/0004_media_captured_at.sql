-- Capture time of each medium (ADR-0015): the local wall-clock moment the photo
-- or video was taken, sent by the client in the captured_at tus metadata as YYYY-MM-DDTHH:MM:SS.
-- NULL when the client could not read it; such media never enter the carousel
-- and never fall back to the file modification time. Existing rows stay NULL.
-- The CHECK validates the shape only; calendar validity is checked by the
-- application before writing.
ALTER TABLE media ADD COLUMN captured_at TEXT NULL
    CHECK (captured_at IS NULL OR (length(captured_at) = 19 AND captured_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]'));

-- Carousel periods and albums: visible media with a capture time, in capture order.
CREATE INDEX media_visible_captured ON media (captured_at, id)
    WHERE visibility = 'visible' AND captured_at IS NOT NULL;
