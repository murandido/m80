-- A ticket that changes after it was issued. The airline moves a segment or a date and
-- charges more miles or money for it, or, rarely, gives some back. Before this migration
-- the only place to put that was the issuance's own points and taxes. But those are what
-- ALTERACAO and the panel's edit overwrite to fix a typo, so the change lost its date and
-- its history. It also landed in the month of the original booking, which may already be
-- closed and settled between the agencies.
--
-- So an alteration is a dated movement hanging off the issuance, and the issuance's own
-- columns keep describing the booking as it was made. Each movement is one more event on
-- the FIFO timeline (internal/inventory), dated by occurred_on, exactly like a
-- cancellation's refund is dated by cancelled_at.
--
-- Additive: a new table and one nullable column. No existing row changes, and nothing
-- computes differently until the first movement is recorded.
CREATE TABLE issuance_adjustments (
    id          UUID NOT NULL DEFAULT gen_random_uuid(),
    issuance_id UUID NOT NULL,
    -- The day the airline charged or returned. It decides which period the movement is
    -- costed in, so it is the one date the closing cares about. created_at is only when it
    -- was typed.
    occurred_on DATE NOT NULL,
    -- Signed. Positive is extra miles the airline charged, and negative is miles it gave
    -- back. One movement can carry both points and money, which is how airlines bill a
    -- change.
    points      INTEGER NOT NULL DEFAULT 0,
    -- Money paid to the airline (positive) or returned by it (negative), in centavos.
    taxes       INTEGER NOT NULL DEFAULT 0,
    -- What the client is charged per thousand of these points, mirroring the issuance's own
    -- column so the sale and the closing price a movement with the formula they already use.
    -- Zero means the agency absorbed the cost: revenue 0 against a real cost, so the profit
    -- drops. On a return it is what the client gets back per thousand. Zero there means the
    -- agency keeps the refund, so the profit rises. The signs follow from points alone, and
    -- nothing downstream needs a special case.
    value_of_the_thousand  INTEGER NOT NULL DEFAULT 0,
    taxes_passed_to_client BOOLEAN NOT NULL DEFAULT false,
    -- Free text, required: the new segment, the date it moved to, the airline's reason.
    -- The flights themselves are edited where they always were, so this is the only record
    -- of what the ticket used to be.
    reason        TEXT NOT NULL,
    created_by_id UUID NOT NULL,
    created_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    -- Soft delete: a movement can sit under a closed period's figures, and the audit log
    -- points at it.
    deleted_at    TIMESTAMP WITH TIME ZONE NULL,
    CONSTRAINT issuance_adjustments_pkey PRIMARY KEY (id),
    CONSTRAINT issuance_adjustments_issuance_fk
        FOREIGN KEY (issuance_id) REFERENCES issuances(id) ON DELETE RESTRICT,
    CONSTRAINT issuance_adjustments_created_by_fk
        FOREIGN KEY (created_by_id) REFERENCES users(id) ON DELETE RESTRICT,
    -- A movement with nothing in it is a mistake, not a record.
    CONSTRAINT issuance_adjustments_not_empty CHECK (points <> 0 OR taxes <> 0),
    CONSTRAINT issuance_adjustments_value_of_the_thousand_non_negative CHECK (value_of_the_thousand >= 0)
);

CREATE INDEX idx_issuance_adjustments_issuance ON issuance_adjustments (issuance_id) WHERE deleted_at IS NULL;

-- Which movement a slice belongs to. NULL is the booking itself, or its cancellation. The
-- closing reads it to price the slice at the movement's rate instead of the issuance's.
-- CASCADE for the same reason as issuance_id here: the slice is derived data, rebuilt by the
-- recompute.
ALTER TABLE point_batch_allocations
    ADD COLUMN adjustment_id UUID NULL
        REFERENCES issuance_adjustments(id) ON DELETE CASCADE;
