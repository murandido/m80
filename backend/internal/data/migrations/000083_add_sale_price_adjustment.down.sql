-- Reverses 000083. Every amount typed in the payments control goes back to the computed one.
ALTER TABLE sales DROP COLUMN IF EXISTS price_adjustment_cents;
