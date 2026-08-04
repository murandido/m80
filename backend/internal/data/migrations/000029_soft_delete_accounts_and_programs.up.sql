-- An account that has ever issued could never be deleted. issuances.account_id is
-- ON DELETE RESTRICT and issuances are soft-deleted, so the row stays in Postgres
-- after being "deleted" and keeps holding the reference - emptying the grid's trash
-- did not help. Purging those issuances is not the way out either: audit_logs and
-- sales reference them with RESTRICT too, so the price would be the audit trail.
--
-- So the account is archived instead of removed, the way issuances and users already
-- are. The DELETE becomes an UPDATE, no foreign key is involved, and a two-year-old
-- issuance still knows which account it was made on.
ALTER TABLE accounts          ADD COLUMN deleted_at TIMESTAMPTZ NULL;
ALTER TABLE account_nicknames ADD COLUMN deleted_at TIMESTAMPTZ NULL;
ALTER TABLE loyalty_programs  ADD COLUMN deleted_at TIMESTAMPTZ NULL;

CREATE INDEX idx_accounts_deleted_at          ON accounts (deleted_at);
CREATE INDEX idx_account_nicknames_deleted_at ON account_nicknames (deleted_at);
CREATE INDEX idx_loyalty_programs_deleted_at  ON loyalty_programs (deleted_at);

-- Both uniqueness rules now apply among the living only. Archiving cannot reserve
-- the alias "heraldo" or the code "SMILES" forever - the operator archived that row
-- precisely so the name could be used again.
DROP INDEX idx_account_nicknames_program_nickname;
CREATE UNIQUE INDEX idx_account_nicknames_program_nickname
    ON account_nicknames (loyalty_program_id, nickname) WHERE deleted_at IS NULL;

ALTER TABLE loyalty_programs DROP CONSTRAINT loyalty_programs_code_unique;
CREATE UNIQUE INDEX idx_loyalty_programs_code
    ON loyalty_programs (code) WHERE deleted_at IS NULL;
