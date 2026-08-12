-- What the agency charges for a thousand miles has always been on the row
-- (value_of_the_thousand). What it *pays* the account's administrator for those
-- same miles never was: that conversion happened outside the system, "per account
-- agreement", which is why the supplier screen could only ever report miles.
--
-- cost_of_the_thousand is the buy side, in centavos, so both halves of the trade
-- live on the issuance they belong to and the payment owed to a supplier can be
-- computed instead of negotiated from a spreadsheet.
--
-- Nullable on purpose. "The cost has not been agreed yet" is a real state and a
-- different one from "it costs nothing" - it is what makes the payment summary
-- refuse to generate rather than quietly under-report what is owed. A default of
-- 0 would have made every issuance ever created look settled.
ALTER TABLE issuances ADD COLUMN cost_of_the_thousand INTEGER NULL;

-- Whether the taxes are part of what is paid for this issuance. It is per issuance and not
-- a setting because it is per deal: some administrators are reimbursed the taxes they paid
-- the airline and some are not, and the same account can have both.
--
-- NOT NULL with a default, unlike the column above, because there is no undecided state
-- here - an issuance either has its taxes reimbursed or it does not, and false is what
-- every existing row means today. Postgres fills a default in the catalogue rather than
-- rewriting the table, so this stays cheap on a table with real data in it.
ALTER TABLE issuances ADD COLUMN taxes_included_in_cost BOOLEAN NOT NULL DEFAULT false;
