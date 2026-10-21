-- Every time somebody pulls an account's password out of the browser extension's vault.
--
-- The panel has always shown passwords decrypted (staff log into the programs by hand), but
-- reading them meant opening accounts one by one. The vault puts every password one click away,
-- which is also what makes copying all of them easy - so each fetch leaves a row here, and the
-- endpoint that writes it is also rate limited per user. Additive only: a new table, nothing
-- existing changes.
CREATE TABLE account_credential_accesses (
    id         UUID PRIMARY KEY,

    -- SET NULL, not CASCADE: an account is hard-deleted only when it has no history, but the
    -- fact that someone read its password is history of its own and must outlive the account.
    account_id UUID NULL REFERENCES accounts(id) ON DELETE SET NULL,

    -- Users are soft-deleted, so RESTRICT never fires in practice; it is there so a hard delete
    -- cannot silently orphan who did it.
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,

    -- What the password was fetched for. TEXT plus a CHECK rather than an ENUM: adding a value
    -- to a Postgres ENUM is a migration the down cannot undo.
    action     TEXT NOT NULL,

    -- The browser address the panel forwarded (see middleware.RealIP). Free text, for a human
    -- reading the trail; nothing parses it.
    client_ip  TEXT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT account_credential_accesses_action_known CHECK (action IN ('copy', 'fill'))
);

-- The two questions the trail answers: who read this account's password, and what did this
-- person read.
CREATE INDEX account_credential_accesses_account_idx ON account_credential_accesses (account_id, created_at DESC);
CREATE INDEX account_credential_accesses_user_idx ON account_credential_accesses (user_id, created_at DESC);
