-- A manual override for how a synced Azul statement line maps to a real issuance in this
-- database. The statement page auto-links a redemption to an issuance by locator (any date,
-- like the batch reconciliation), but the operator can pin a different one - or link one the
-- auto match missed - after turning on edit mode. This table holds only those manual pins;
-- the auto match is computed, not stored. Additive and empty on creation: with no rows the
-- resolver falls straight through to the auto match, so this migration changes no number.
CREATE TABLE azul_booking_links (
    -- One manual link per (account, locator): a locator identifies a single booking on an
    -- account's statement, so pinning it twice makes no sense. The composite key is the
    -- natural one and is also the access path (the resolver looks a batch of locators up by
    -- account).
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    locator    TEXT NOT NULL,

    -- The issuance the operator pinned this locator to. CASCADE so a hard-deleted issuance
    -- takes its manual links with it - a dangling link would resolve to nothing anyway.
    -- Issuances are normally soft-deleted, so this only fires on a true removal.
    issuance_id UUID NOT NULL REFERENCES issuances(id) ON DELETE CASCADE,

    -- Who made the pin, for audit. SET NULL rather than CASCADE: losing the user must not lose
    -- the link, only the attribution. Nullable because a link can outlive the account of the
    -- person who made it.
    created_by UUID NULL REFERENCES users(id) ON DELETE SET NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (account_id, locator)
);
