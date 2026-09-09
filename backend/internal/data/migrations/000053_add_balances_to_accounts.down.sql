-- Loses the last balance read off each airline's site. Nothing computes from it - the
-- expected balance is derived from the batches and the issuances - so what goes is the
-- comparison, not any figure the costing depends on.
ALTER TABLE accounts DROP CONSTRAINT IF EXISTS accounts_real_balance_not_negative;
ALTER TABLE accounts DROP COLUMN IF EXISTS real_balance_checked_on;
ALTER TABLE accounts DROP COLUMN IF EXISTS real_balance_points;
