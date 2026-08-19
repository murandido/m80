-- Dropping the column discards which issuances had already been checked. There is
-- nowhere else that was recorded, so going back down means the sweep starts over.
ALTER TABLE issuances DROP COLUMN IF EXISTS reviewed;
