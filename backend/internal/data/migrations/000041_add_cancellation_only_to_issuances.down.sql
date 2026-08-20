-- Dropping this loses which cancelled issuances were never really issued, and the supplier
-- balance goes back to netting those days to zero instead of showing the refund the sheet
-- shows. Nothing else records the distinction.
ALTER TABLE issuances DROP COLUMN IF EXISTS cancellation_only;
