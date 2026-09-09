-- A stretch of an account's costing that has been settled and may no longer be recomputed.
--
-- Closing exists because the allocations are derived: every recompute rebuilds them from
-- the batches and the issuances, which is what keeps the costing correct while corrections
-- and late entries arrive. That is exactly what must NOT happen to a month whose profit
-- was already split with the other agency and paid - so a closed period freezes, and the
-- recompute starts after it.
--
-- Closing is an act, never a date rolling over. A month ends on the calendar whether or
-- not anybody has finished checking it, and a period that closed itself would freeze
-- figures mid-review.
CREATE TABLE inventory_periods (
    id           UUID NOT NULL DEFAULT gen_random_uuid(),
    account_id   UUID NOT NULL,
    starts_on    DATE NOT NULL,
    ends_on      DATE NOT NULL,
    -- NULL is a period somebody opened but has not settled. Only a non-null closed_at
    -- freezes anything.
    closed_at    TIMESTAMP WITH TIME ZONE NULL,
    closed_by_id UUID NULL,
    created_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT inventory_periods_pkey PRIMARY KEY (id),
    CONSTRAINT inventory_periods_account_fk
        FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE RESTRICT,
    CONSTRAINT inventory_periods_closed_by_fk
        FOREIGN KEY (closed_by_id) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT inventory_periods_ordered CHECK (ends_on >= starts_on)
);

-- The one read there is: how far this account is frozen, which every recompute asks first.
CREATE INDEX idx_inventory_periods_closed
    ON inventory_periods (account_id, ends_on)
    WHERE closed_at IS NOT NULL;
