-- Restoring the global unique name fails on purpose once two agencies each have a client of
-- the same name: there is no way back that does not merge or rename somebody's client, and
-- that is a decision for a person, not a migration. Rename one of them, then run this again.
DROP INDEX IF EXISTS idx_clients_name_without_agency;
DROP INDEX IF EXISTS idx_clients_agency_name;
ALTER TABLE clients ADD CONSTRAINT clients_name_unique UNIQUE (name);

DROP INDEX IF EXISTS idx_clients_agency_id;
ALTER TABLE clients DROP COLUMN IF EXISTS agency_id;
