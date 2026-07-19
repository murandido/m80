-- ON DELETE SET NULL, not RESTRICT: account_nicknames already cascades from accounts
-- (migration 000008), so RESTRICT here would block deleting any account that has both
-- a nickname and an issuance referencing it.
ALTER TABLE issuances
    ADD COLUMN account_nickname_id UUID NULL,
    ADD CONSTRAINT issuances_account_nickname_fk
        FOREIGN KEY (account_nickname_id) REFERENCES account_nicknames(id) ON DELETE SET NULL;
