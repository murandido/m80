-- Archived people come back as live ones: the marker is all this drops.
DROP INDEX IF EXISTS idx_people_deleted_at;
ALTER TABLE people DROP COLUMN IF EXISTS deleted_at;
