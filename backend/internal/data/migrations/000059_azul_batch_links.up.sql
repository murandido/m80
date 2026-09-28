-- A manual override for how a synced Azul point-entry line (a purchase or an accrual) maps to a
-- real point batch (lote) in this database. The statement page auto-matches an entry to the
-- batch it was recorded into - a batch keeps the credits it was summed from - but the operator
-- can pin a different one, or link one the auto match missed, after turning on edit mode. This
-- table holds only those manual pins; the auto match is computed, not stored. Additive and empty
-- on creation, so this migration changes no number.
--
-- The sibling of azul_booking_links (migration 58): redemptions link to issuances, point entries
-- link to batches. Same shape, keyed by the same line key.
CREATE TABLE azul_batch_links (
    -- One manual link per (account, line key). The key is the entry's locator when it has one,
    -- otherwise a content signature - the same key azul_booking_links uses, so a line is pinned
    -- by the identity it shows on the page.
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    line_key   TEXT NOT NULL,

    -- The batch the operator pinned this entry to. CASCADE so a deleted batch takes its manual
    -- links with it - a dangling link would resolve to nothing anyway.
    batch_id UUID NOT NULL REFERENCES point_batches(id) ON DELETE CASCADE,

    -- Who made the pin, for audit. SET NULL rather than CASCADE: losing the user must not lose
    -- the link, only the attribution.
    created_by UUID NULL REFERENCES users(id) ON DELETE SET NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (account_id, line_key)
);
