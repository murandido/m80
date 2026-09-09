-- What the airline itself says the account has, and when somebody last looked.
--
-- The panel can derive the balance it *expects* - the batches minus what was issued - but
-- it has no way to know the real one. Neither export carries a position: the Azul CSV and
-- the Smiles HTML are movement lists, so the reconciliation compares the *change* over a
-- period and never the number on the account's own page. Somebody has to read it there and
-- write it down, and that is what this column is.
--
-- Only the latest reading, not a history. The two figures side by side answer the question
-- being asked today - do they agree? - and a dated series is the shape the balance
-- monitoring will need when it is built, not before.
ALTER TABLE accounts ADD COLUMN real_balance_points INTEGER NULL;

-- The day the reading was taken, which is what makes it worth anything: a balance without
-- a date cannot be compared to a panel that has moved since.
ALTER TABLE accounts ADD COLUMN real_balance_checked_on DATE NULL;

-- A balance is a count of points, never negative. NULL is "nobody has looked", which is
-- the state of every account until somebody does.
ALTER TABLE accounts ADD CONSTRAINT accounts_real_balance_not_negative
    CHECK (real_balance_points IS NULL OR real_balance_points >= 0);
