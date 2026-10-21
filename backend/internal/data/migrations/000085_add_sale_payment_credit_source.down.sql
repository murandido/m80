-- Reverses 000085. The transfer lines are derived, so they go rather than turning into lines
-- that look typed on their sale. sales.paid of the sales they touched is left stale until the
-- next replay of their client recomputes it.
DELETE FROM sale_payments WHERE credit_from_sale_id IS NOT NULL;
DROP INDEX IF EXISTS idx_sale_payments_credit_from_sale;
ALTER TABLE sale_payments DROP CONSTRAINT IF EXISTS sale_payments_credit_from_sale_fk;
ALTER TABLE sale_payments DROP COLUMN IF EXISTS credit_from_sale_id;
