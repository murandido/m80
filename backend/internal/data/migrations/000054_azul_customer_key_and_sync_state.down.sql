-- Reverses 000054. Drops the sync bookkeeping and the stored customerKey. Nothing in the
-- costing derives from either - the sync only ever fills accounts.real_balance_points, which
-- predates this migration - so what is lost is the ability to sync, not any settled figure.
DROP TABLE IF EXISTS azul_sync_state;
DROP TYPE IF EXISTS azul_sync_state_kind;
ALTER TABLE accounts DROP COLUMN IF EXISTS azul_customer_key;
