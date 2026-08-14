-- Dropping these loses which issuances were imported, which is the fact that keeps Traveos
-- from being asked to emit a ticket for them. Only run this while the feature is still
-- unused - once a spreadsheet day has been imported, the column is protecting real rows.
ALTER TABLE issuances DROP COLUMN IF EXISTS traveos_attempted_at;
ALTER TABLE issuances DROP COLUMN IF EXISTS imported;
