-- Recreates the columns exactly as migration 40 declared them, empty. The values are not
-- recoverable from here: restore them from the dump taken before the up migration ran.
ALTER TABLE loyalty_programs ADD COLUMN IF NOT EXISTS redemption_fee_domestic_cents      INTEGER NULL;
ALTER TABLE loyalty_programs ADD COLUMN IF NOT EXISTS redemption_fee_international_cents INTEGER NULL;
ALTER TABLE loyalty_programs ADD COLUMN IF NOT EXISTS redemption_fee_advance_days        INTEGER NULL;

ALTER TABLE accounts ADD COLUMN IF NOT EXISTS waives_redemption_fee BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE issuances ADD COLUMN IF NOT EXISTS redemption_fee_saved_cents INTEGER NULL;
ALTER TABLE issuances ADD COLUMN IF NOT EXISTS redemption_fee_manual      BOOLEAN NOT NULL DEFAULT FALSE;
