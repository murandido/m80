-- Azul stopped charging the redemption fee on Diamante Unique accounts, so the control that
-- migration 40 added is gone from the code and these six columns have no reader or writer
-- left. They were kept for one release on purpose: an image rollback does not undo schema,
-- and the previous image still selected them.
--
-- Destructive. Take a dump (make db-backup) before this runs: the down migration recreates
-- the columns but not what they held - the saved fees, the manual corrections, which
-- accounts were marked exempt and the programs' fee settings come back only from the dump.
--
-- IF EXISTS so a database that already lost a column by hand does not stop the boot.
ALTER TABLE issuances DROP COLUMN IF EXISTS redemption_fee_manual;
ALTER TABLE issuances DROP COLUMN IF EXISTS redemption_fee_saved_cents;

ALTER TABLE accounts DROP COLUMN IF EXISTS waives_redemption_fee;

ALTER TABLE loyalty_programs DROP COLUMN IF EXISTS redemption_fee_advance_days;
ALTER TABLE loyalty_programs DROP COLUMN IF EXISTS redemption_fee_international_cents;
ALTER TABLE loyalty_programs DROP COLUMN IF EXISTS redemption_fee_domestic_cents;
