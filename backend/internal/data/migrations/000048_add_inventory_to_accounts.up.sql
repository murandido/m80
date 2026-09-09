-- Turns on the points inventory for one account, and nothing else.
--
-- Two different things are called "cost" in this system, and this flag is what tells them
-- apart. On an ordinary account the agency buys points from the account's administrator,
-- and issuances.cost_of_the_thousand is that transfer price - typed by hand on
-- /sales/suppliers. On these accounts there is no administrator to pay: the points were
-- bought by us (a Livelo transfer, a bank campaign, a direct purchase), so the cost is an
-- acquisition cost, and it belongs to a batch rather than to any single issuance.
--
-- FALSE on every existing row, which is what makes this migration inert: nothing computes
-- differently until somebody marks an account, so the whole feature can ship to production
-- without changing a number.
--
-- Deliberately NOT derived from accounts.category = 'injunction' (migration 37). Those two
-- happen to overlap today, but they answer different questions: the category says whether
-- the program's beneficiary cap applies, this says who paid for the points. An injunction
-- account whose points we did not buy would get the wrong costing.
ALTER TABLE accounts ADD COLUMN owns_point_inventory BOOLEAN NOT NULL DEFAULT FALSE;

-- The share of a batch's profit that goes to an agency other than the one that funded it,
-- as a whole percent. It is only a default: every batch freezes its own copy at creation
-- (migration 49), so changing the arrangement never rewrites a month that was already
-- settled and paid.
--
-- Zero is the honest default for the accounts that have no arrangement at all - the profit
-- stays whole with whoever funded the batch.
ALTER TABLE accounts ADD COLUMN default_profit_share_percent SMALLINT NOT NULL DEFAULT 0;
