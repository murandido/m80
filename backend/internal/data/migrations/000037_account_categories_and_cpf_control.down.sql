-- Safe to run: nothing outside this migration depends on any of it. An older API image
-- neither reads the category nor counts beneficiaries, so the screens go back to what they
-- showed before. What is lost is the beneficiary ledger itself, including any manual
-- correction an operator made - the automatic part rebuilds from the issuances on the next
-- up, the manual part does not. Take a dump first if that matters.
DROP TABLE IF EXISTS account_beneficiary_uses;
DROP TABLE IF EXISTS account_beneficiaries;

ALTER TABLE loyalty_programs DROP COLUMN IF EXISTS cpf_limit_mode;
ALTER TABLE loyalty_programs DROP COLUMN IF EXISTS cpf_limit_window;
ALTER TABLE loyalty_programs DROP COLUMN IF EXISTS cpf_limit;
ALTER TABLE accounts DROP COLUMN IF EXISTS category;

-- After the columns, never before: a type still in use by a column cannot be dropped.
DROP TYPE IF EXISTS cpf_limit_mode;
DROP TYPE IF EXISTS cpf_limit_window;
DROP TYPE IF EXISTS account_category;
