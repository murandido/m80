-- Whether an account's statement is read from the airline at all.
--
-- Until now that was decided by owns_point_inventory, because the only reason to read a
-- statement was to price the points in it. That conflated two different questions: "are these
-- points ours to cost?" and "do we want to see what happened in this account?". An account
-- administered for someone else has a statement worth watching - the redemptions in it are our
-- own issuances - and no inventory to cost.
--
-- So the two questions get two columns. This one governs connecting the account, the periodic
-- sync, and the statement screens. owns_point_inventory keeps governing everything that prices
-- points: batches, the closing period and the FIFO costing. An account can have either, both or
-- neither.
--
-- Additive and inert: NOT NULL with a default, then backfilled to exactly what the old rule
-- decided, so no existing account changes behaviour on deploy. What changes is that an operator
-- can now turn it on for an account the old rule refused.
ALTER TABLE accounts ADD COLUMN tracks_statement BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE accounts SET tracks_statement = owns_point_inventory;
