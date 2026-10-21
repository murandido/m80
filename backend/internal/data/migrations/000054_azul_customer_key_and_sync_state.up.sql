-- The two things the Azul statement sync needs to keep per account: which account to read,
-- and how the last read went. Both are additive - a nullable column and a new table - so this
-- migration is inert until the sync is switched on: no existing row changes and nothing
-- computes differently.

-- The customerKey is the opaque hash the airline's API uses to select an account. It is the
-- only account selector the read calls take, and it is discovered once, by logging the account
-- in through a real browser. Stored encrypted at rest by the same BeforeSave/AfterFind hooks as
-- accounts.password (AES-256-GCM, internal/utils/crypto.go), so the column holds "enc:..." and
-- is never returned by the API. NULL means the account has not been connected yet.
--
-- TEXT, not a fixed width: the ciphertext is longer than the key and its length is not ours to
-- assume.
ALTER TABLE accounts ADD COLUMN azul_customer_key TEXT NULL;

-- The lifecycle of one account's sync, as a small state machine the panel reads.
--   pending          - connected, never synced yet
--   ok               - last round matched the balance and the window was stored
--   error            - a transport/credential failure; retried on the next tick
--   inconsistent     - the statement's last-day balance did not match points/resume, so the
--                      window was NOT stored (the balance is the absolute truth); shown for a
--                      human to look at
--   needs_attention  - login has failed enough times that the stored credentials are suspect;
--                      this is the only state the panel raises for manual intervention
CREATE TYPE azul_sync_state_kind AS ENUM
    ('pending', 'ok', 'error', 'inconsistent', 'needs_attention');

CREATE TABLE azul_sync_state (
    -- One row per account, keyed by the account itself: there is exactly one sync lifecycle
    -- per account, so the account id is the natural primary key. CASCADE because this row is
    -- derived, disposable state - if an account is ever truly removed, its sync bookkeeping
    -- goes with it.
    account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,

    -- When the last successful read happened, and the balance it saw. last_balance_points is
    -- how the ticker decides whether to pull the statement at all: the balance is polled every
    -- round, the statement only when it has moved. NULL on both until the first success.
    last_synced_at        TIMESTAMPTZ NULL,
    last_balance_points   INTEGER NULL,

    state                 azul_sync_state_kind NOT NULL DEFAULT 'pending',

    -- How many times in a row the sync (or the login) has failed. Drives the backoff and the
    -- threshold that flips the state to needs_attention; reset to zero on any success.
    consecutive_failures  INTEGER NOT NULL DEFAULT 0,

    -- A short human-readable note about the current state (the last error, or the day a
    -- balance mismatch was seen). Free text, for the panel to show; nothing parses it.
    detail                TEXT NULL,

    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT azul_sync_state_failures_not_negative CHECK (consecutive_failures >= 0),
    CONSTRAINT azul_sync_state_balance_not_negative
        CHECK (last_balance_points IS NULL OR last_balance_points >= 0)
);
