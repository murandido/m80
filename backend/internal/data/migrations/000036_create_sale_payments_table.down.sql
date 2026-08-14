-- sales.paid survives this table and is kept in sync with it by the API, so dropping the
-- ledger loses the detail (how much, when, who) but not which sales are settled. That is
-- what makes the down migration safe to run: the older API reads sales.paid and is right.
DROP TABLE IF EXISTS sale_payments;
