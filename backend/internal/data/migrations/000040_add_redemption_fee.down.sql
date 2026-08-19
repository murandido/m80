-- Dropping these discards every saving already computed or corrected by hand, and which
-- accounts were marked exempt. Neither is recorded anywhere else, so going back down means
-- the whole period has to be recalculated - and the manual corrections are simply gone.
ALTER TABLE issuances DROP COLUMN IF EXISTS redemption_fee_manual;
ALTER TABLE issuances DROP COLUMN IF EXISTS redemption_fee_saved_cents;

ALTER TABLE accounts DROP COLUMN IF EXISTS waives_redemption_fee;

ALTER TABLE loyalty_programs DROP COLUMN IF EXISTS redemption_fee_advance_days;
ALTER TABLE loyalty_programs DROP COLUMN IF EXISTS redemption_fee_international_cents;
ALTER TABLE loyalty_programs DROP COLUMN IF EXISTS redemption_fee_domestic_cents;
