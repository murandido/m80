-- Drops the spread lines first: without their lump sum they would stay on their sales as
-- if typed there, and the sales would keep reading as paid.
DELETE FROM sale_payments WHERE client_payment_id IS NOT NULL;
ALTER TABLE sale_payments DROP COLUMN IF EXISTS client_payment_id;
DROP TABLE IF EXISTS client_payments;
