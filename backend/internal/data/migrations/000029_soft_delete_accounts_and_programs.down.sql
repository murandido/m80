-- This rollback can fail, and that is deliberate. Recreating the total uniqueness
-- indexes will be refused by Postgres if an archived row shares a code or an alias
-- with a living one - which is exactly the situation the partial indexes were added
-- to allow. Dropping the archived rows to force it through would decide in silence
-- which of the two survives, so the rollback is left to say so out loud instead.
DROP INDEX idx_loyalty_programs_code;
ALTER TABLE loyalty_programs ADD CONSTRAINT loyalty_programs_code_unique UNIQUE (code);

DROP INDEX idx_account_nicknames_program_nickname;
CREATE UNIQUE INDEX idx_account_nicknames_program_nickname
    ON account_nicknames (loyalty_program_id, nickname);

DROP INDEX idx_loyalty_programs_deleted_at;
DROP INDEX idx_account_nicknames_deleted_at;
DROP INDEX idx_accounts_deleted_at;

ALTER TABLE loyalty_programs  DROP COLUMN deleted_at;
ALTER TABLE account_nicknames DROP COLUMN deleted_at;
ALTER TABLE accounts          DROP COLUMN deleted_at;
