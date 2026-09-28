-- The synced statement itself: one row per movement of the accounts whose points are ours,
-- read from the Azul API instead of a hand-exported CSV. Additive and empty on creation - the
-- sync fills it - so this migration changes no existing number.
--
-- Unlike internal/statement, which reads a file and drops it, this IS stored: the API is polled
-- on a ticker and the panel needs the last window to be there between polls. Storage is safe
-- here where it was not for the CSV precisely because the source is now trusted against the
-- balance - a window is only written when its last-day balance matches points/resume.
CREATE TABLE azul_statement_lines (
    id         UUID PRIMARY KEY,
    -- CASCADE: these lines are a cache of the airline's statement, not operational records. If
    -- an account is ever truly removed, its synced statement goes with it.
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,

    -- The day the movement happened, which is the grain everything is keyed and replaced on:
    -- the legacy endpoint only gives day precision. occurred_at is kept for a future source
    -- with a real clock and is left NULL until one exists - it is not a midnight stand-in.
    occurred_on DATE NOT NULL,
    occurred_at TIMESTAMPTZ NULL,

    -- kind is the movement in the panel's terms (internal/statement.Kind): redemption, refund
    -- or accrual reconcile; 'expiration' is carried but kept OUT of the FIFO for now (shown in
    -- reconciliation only); 'other' is anything the mapping does not recognise. TEXT rather
    -- than an enum because the airline's type codes are not fully enumerated and a new one must
    -- not need a migration to be stored.
    kind             TEXT NOT NULL,
    -- The airline's own type code (e.g. AI accrual, AN redemption, PE expiration) and its
    -- label, kept verbatim so a human can see what the mapping was made from.
    type_code        TEXT NULL,
    type_description TEXT NULL,

    -- Signed, carrying the airline's own sign: negative when points left the account. The sum
    -- of a window is its change in balance.
    points INTEGER NOT NULL,

    -- Booking reference and route, parsed from the free-text comments of a direct redemption
    -- (empty on everything else). partner_name and description are the airline's own wording.
    locator      TEXT NULL,
    origin       TEXT NULL,
    destination  TEXT NULL,
    partner_name TEXT NULL,
    description  TEXT NULL,

    -- The end-of-day balance the airline reported for occurred_on. The last day's day_balance
    -- is what the sync validates against points/resume before storing the window.
    day_balance INTEGER NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The window is always read and replaced per account and by day, so this is the access path:
-- delete occurred_on >= window-start for one account, reinsert, and read it back ordered.
CREATE INDEX idx_azul_statement_lines_account_day
    ON azul_statement_lines (account_id, occurred_on);
