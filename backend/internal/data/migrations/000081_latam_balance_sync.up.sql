-- What reading a LATAM Pass balance needs to keep per account: the session it renews and how the
-- last read went. All additive - one nullable column, one enum and one new table - so this
-- migration is inert until somebody links a LATAM account: no existing row changes and nothing
-- computes differently.
--
-- It is migration 63 (Smiles) minus the statement. LATAM is read for its balance only: the
-- balance itself lands in accounts.real_balance_points like Azul's and Smiles', so nothing here
-- stores a figure. The contract is in internal/latam/account.go.

-- The refresh token is the whole of a linked LATAM account: it renews the session without
-- anyone logging in again, and whoever holds it can read the account until it is revoked. So it
-- is encrypted at rest by the same BeforeSave/AfterFind hooks as accounts.password (AES-256-GCM,
-- internal/utils/crypto.go), the column holds "enc:...", and it is never returned by the API.
--
-- Auth0 may rotate it on a renewal, so it is rewritten whenever a new one comes back. NULL means
-- the account has not been linked, or was disconnected. TEXT because the ciphertext's length is
-- not ours to assume.
ALTER TABLE accounts ADD COLUMN latam_refresh_token TEXT NULL;

-- The lifecycle of one account's balance read, as a small state machine the panel reads.
--   pending          - linked, never read yet
--   ok               - the last round read the balance
--   error            - a transport failure; retried on the next tick
--   needs_login      - the stored session was refused. Only a new login on LATAM's own page
--                      fixes it (the tenant refuses password logins for the app), so it is its
--                      own state and not an error to retry
--   needs_attention  - enough rounds have failed in a row that the account is worth looking at
CREATE TYPE latam_sync_state_kind AS ENUM
    ('pending', 'ok', 'error', 'needs_login', 'needs_attention');

CREATE TABLE latam_sync_state (
    -- One row per account, keyed by the account itself. CASCADE because this row is derived,
    -- disposable state.
    account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,

    -- When the last successful read happened. NULL until the first success. There is no
    -- balance column: the figure goes to accounts.real_balance_points, and with no statement to
    -- pull there is nothing for a stored balance to decide.
    last_synced_at       TIMESTAMPTZ NULL,

    state                latam_sync_state_kind NOT NULL DEFAULT 'pending',

    -- How many rounds have failed in a row. Drives the threshold that flips the state to
    -- needs_attention; reset to zero on any success.
    consecutive_failures INTEGER NOT NULL DEFAULT 0,

    -- A short human-readable note about the current state. Free text, for the panel to show;
    -- nothing parses it.
    detail               TEXT NULL,

    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT latam_sync_state_failures_not_negative CHECK (consecutive_failures >= 0)
);
