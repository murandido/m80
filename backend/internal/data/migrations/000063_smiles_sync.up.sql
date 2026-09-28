-- Everything the Smiles statement sync needs to keep per account: the session it renews, how
-- the last read went, and the statement itself. All additive - one nullable column and two new
-- tables - so this migration is inert until the sync is switched on: no existing row changes
-- and nothing computes differently.
--
-- The shape deliberately mirrors migrations 54 and 55 (Azul), because the panel reconciles both
-- programmes through the same core. Where it differs, it differs for a measured reason, noted
-- at the point of difference. The contract this implements is in backend/docs/smiles-api.md.

-- The refresh token is the whole of a connected Smiles account. Unlike Azul's customerKey it is
-- a live credential, not an identifier: it renews the session without a second factor, and
-- whoever holds it can read the account until it is revoked. So it is encrypted at rest by the
-- same BeforeSave/AfterFind hooks as accounts.password (AES-256-GCM, internal/utils/crypto.go),
-- the column holds "enc:...", and it is never returned by the API.
--
-- It is also rewritten on every sync round: the tenant rotates the token on each renewal and
-- refuses the one it replaced. NULL means the account has not been connected, or was
-- disconnected.
--
-- TEXT, not a fixed width: the ciphertext is longer than the token and its length is not ours
-- to assume.
ALTER TABLE accounts ADD COLUMN smiles_refresh_token TEXT NULL;

-- The lifecycle of one account's sync, as a small state machine the panel reads.
--   pending          - connected, never synced yet
--   ok               - the last round read the balance and stored the window
--   error            - a transport failure; retried on the next tick
--   needs_mfa        - the stored session was refused and cannot be renewed. Only a human
--                      typing a new verification code fixes it, which is why it is its own
--                      state and not an error: the sync must never answer it by replaying the
--                      password, because repeated password attempts are how an account gets
--                      locked
--   needs_attention  - enough rounds have failed in a row that the account is worth looking at
--
-- There is no 'inconsistent' here, and that is the one real difference from Azul. Azul refuses
-- to store a window whose last day does not add up to the balance; Smiles publishes no
-- end-of-day balance anywhere - not in the statement, not in the profile - so there is nothing
-- to check a window against. Inventing a substitute would be inventing confidence.
CREATE TYPE smiles_sync_state_kind AS ENUM
    ('pending', 'ok', 'error', 'needs_mfa', 'needs_attention');

CREATE TABLE smiles_sync_state (
    -- One row per account, keyed by the account itself: there is exactly one sync lifecycle per
    -- account. CASCADE because this row is derived, disposable state.
    account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,

    -- When the last successful read happened, and the balance it saw. last_balance_miles is how
    -- the ticker decides whether to pull the statement at all: the balance is read every round,
    -- the statement only when it has moved. NULL on both until the first success.
    last_synced_at      TIMESTAMPTZ NULL,
    last_balance_miles  INTEGER NULL,

    state               smiles_sync_state_kind NOT NULL DEFAULT 'pending',

    -- How many rounds have failed in a row. Drives the threshold that flips the state to
    -- needs_attention; reset to zero on any success.
    consecutive_failures INTEGER NOT NULL DEFAULT 0,

    -- A short human-readable note about the current state. Free text, for the panel to show;
    -- nothing parses it.
    detail              TEXT NULL,

    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT smiles_sync_state_failures_not_negative CHECK (consecutive_failures >= 0),
    CONSTRAINT smiles_sync_state_balance_not_negative
        CHECK (last_balance_miles IS NULL OR last_balance_miles >= 0)
);

-- The synced statement: one row per movement of the accounts whose miles are ours, read from
-- the Smiles API instead of an HTML page saved by hand. Additive and empty on creation.
CREATE TABLE smiles_statement_lines (
    id         UUID PRIMARY KEY,
    -- CASCADE: these lines are a cache of the airline's statement, not operational records.
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,

    -- The day the movement happened. The payload carries a clock too, but in a timezone the
    -- contract never states, so occurred_at is left NULL rather than filled with a wall time
    -- nobody can place - a null is honest, a wrong timestamp is not.
    occurred_on DATE NOT NULL,
    occurred_at TIMESTAMPTZ NULL,

    -- kind is the movement in the panel's terms (internal/statement.Kind): redemption, refund,
    -- accrual and purchase reconcile; 'other' is anything the mapping does not recognise. TEXT
    -- rather than an enum, for the same reason as Azul's: a product name the mapping has not
    -- seen must not need a migration to be stored.
    kind             TEXT NOT NULL,
    -- The airline's own classification, kept verbatim so a human can see what the mapping was
    -- made from. type_code is "Redemption" or "Accrual"; sub_type is what the movement's legs
    -- agree on, and "Cancellation" there is the only way this API spells an estorno.
    type_code        TEXT NULL,
    sub_type         TEXT NULL,
    type_description TEXT NULL,

    -- Signed with OUR sign, not the airline's: the payload is unsigned throughout, so a
    -- redemption is negated on the way in and the sum of a window is its change in balance.
    points INTEGER NOT NULL,

    -- Booking reference and route, empty on a credit that is not a flight. origin/destination
    -- are IATA codes straight from the payload - Smiles gives the codes, so there is no
    -- city-to-IATA table to maintain as there is for Azul (see azul_city_codes).
    locator      TEXT NULL,
    origin       TEXT NULL,
    destination  TEXT NULL,
    partner_name TEXT NULL,
    description  TEXT NULL,
    -- Where the movement was made ("Web", "APP", "MSR"). It is what tells an operator's own
    -- booking from one the airline generated.
    channel      TEXT NULL,

    -- The balance the account held at the end of occurred_on.
    --
    -- Read this as DERIVED, not reported. Smiles publishes no closing balance anywhere, so the
    -- sync computes it: the balance it read is the window's end, and walking the days backwards
    -- by their own movements gives each earlier day. That makes it useful to look at and useless
    -- to check against - it adds up by construction, so it can never disagree with the movements
    -- it was built from. Azul's day_balance is the opposite: the airline's own figure, which is
    -- exactly why a mismatch there means a movement is missing. Nothing may validate a window
    -- against this column.
    --
    -- NULL on a line whose round could not establish an opening balance.
    day_balance INTEGER NULL,

    -- The airline's own identifier for the movement: the booking reference on a flight, the
    -- transaction number on a credit. Azul has no such field and pays for it with a synthetic
    -- key that changes whenever a window is replaced; here a line keeps its identity across
    -- syncs, which is what makes a manual link survive.
    --
    -- Nullable, and NOT unique: it was unique across the measured year, but a uniqueness
    -- constraint on a third party's identifier turns their surprise into our failed sync.
    external_id TEXT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The window is always read and replaced per account and by day, so this is the access path:
-- delete occurred_on >= window-start for one account, reinsert, and read it back ordered.
CREATE INDEX idx_smiles_statement_lines_account_day
    ON smiles_statement_lines (account_id, occurred_on);
