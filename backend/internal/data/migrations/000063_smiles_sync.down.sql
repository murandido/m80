-- Reverses 000063. Drops the synced statement cache, the sync bookkeeping and the stored
-- session.
--
-- What is lost is the ability to sync, not a settled figure: the statement is a cache the API
-- can be asked for again, and the costing derives from point_batches, not from these rows. The
-- one real cost is the session - dropping the refresh token means the account has to be
-- connected again, with a fresh verification code typed by a human.
DROP TABLE IF EXISTS smiles_statement_lines;
DROP TABLE IF EXISTS smiles_sync_state;
DROP TYPE IF EXISTS smiles_sync_state_kind;
ALTER TABLE accounts DROP COLUMN IF EXISTS smiles_refresh_token;
