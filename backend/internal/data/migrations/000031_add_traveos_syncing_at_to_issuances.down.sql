DROP INDEX IF EXISTS idx_issuances_traveos_syncing_at;
ALTER TABLE issuances DROP COLUMN traveos_syncing_at;
