-- A login for an outside customer who only reads the Azul/Smiles statement of the accounts
-- granted to them (user_accounts, migration 089). Kept in its own migration because a value
-- added to an enum cannot be used in the same transaction that adds it.
-- Append-only, no table rewrite; IF NOT EXISTS keeps it idempotent like 000032.
ALTER TYPE user_role ADD VALUE IF NOT EXISTS 'statement_client';
