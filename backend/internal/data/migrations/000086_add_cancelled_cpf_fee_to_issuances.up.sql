-- Cancelling a booking gives the miles back to the supplier's account, but not the CPFs it
-- used: the programme keeps them counted against the account's passenger limit, and some
-- suppliers charge for that. This is the fee the agency owes the supplier for it, in
-- centavos. It is supplier control only - the client is usually charged separately, through
-- cancellation_fee - and it adds to what the supplier balance and payment summary say the
-- agency pays.
--
-- Additive and nullable: NULL means the supplier charged nothing, which is every existing
-- cancellation and most new ones.
ALTER TABLE issuances ADD COLUMN cancelled_cpf_fee INTEGER NULL;
