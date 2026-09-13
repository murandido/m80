-- A free-text line per sale for the payments screen - "paga na sexta", "cobrar junto com a
-- volta". It is about the money between the agency and the client, which is why it lives on
-- the sale and not in issuances.observation: that one is about the booking, and is read on
-- another screen by other people.
--
-- Nullable and without a default, so adding it is a metadata-only change on every row that
-- already exists. NULL is "nobody wrote anything", which is what every sale meant until now.
ALTER TABLE sales ADD COLUMN note TEXT NULL;
