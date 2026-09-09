-- Which batch paid for which issuance, and what that slice cost.
--
-- This is a derived table: it is rebuilt from the batches and the issuances, never typed.
-- It exists because the cost of one issuance is not a single number. A booking that does
-- not fit in the oldest batch is split across two, at two different rates - the hand-kept
-- spreadsheet wrote exactly that, one line per slice:
--
--   MJF6HC  -108,202 milheiros  @ 15,000000  ->  R$ 1.623,03   (RESTO ULTIMA)
--   MJF6HC  -251,798 milheiros  @ 13,580006  ->  R$ 3.419,42   (ESFERA)
--
-- Collapsing that into one average rate on the issuance is what the whole table is here to
-- avoid: 5.042,45 of real cost becomes 5.043,60 once the average is rounded to a whole
-- centavo per thousand, and the error compounds over a month of bookings.
CREATE TABLE point_batch_allocations (
    id          UUID NOT NULL DEFAULT gen_random_uuid(),
    issuance_id UUID NOT NULL,
    -- NULL is the pending state: the issuance consumed points no batch has been entered
    -- for yet. The costing never blocks an issuance - the bot writes bookings in real time
    -- and the purchase is recorded whenever it is recorded - so the shortfall is reported
    -- rather than refused, and resolves itself when the missing batch is finally entered.
    batch_id    UUID NULL,
    -- The day this slice belongs to: the issuance's created_at when points are consumed,
    -- the refund's date when they are given back. It is what decides which period the
    -- slice falls in, which is why a reversal can land in an open month while the booking
    -- it reverses stays in a closed one.
    occurred_on DATE    NOT NULL,
    -- Negative on a reversal. Keeping returns in the same table as consumption is what
    -- makes a batch's remaining balance a plain SUM, and lets an exhausted batch come back
    -- to life without any of its history being rewritten.
    points      INTEGER NOT NULL,
    cost_cents  INTEGER NOT NULL,
    created_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT point_batch_allocations_pkey PRIMARY KEY (id),
    -- CASCADE, unlike every other reference to issuances in this schema: an allocation is
    -- derived data with no independent meaning, so when the issuance it prices is really
    -- gone there is nothing left to preserve. Issuances are soft-deleted, so this fires
    -- only on a hard delete.
    CONSTRAINT point_batch_allocations_issuance_fk
        FOREIGN KEY (issuance_id) REFERENCES issuances(id) ON DELETE CASCADE,
    -- RESTRICT, and the reason point_batches is soft-deleted: a batch that priced anything
    -- can never be really deleted, only archived.
    CONSTRAINT point_batch_allocations_batch_fk
        FOREIGN KEY (batch_id) REFERENCES point_batches(id) ON DELETE RESTRICT
);

-- The two reads this table has. By batch to know what a batch has left and what it earned;
-- by issuance to price one booking.
CREATE INDEX idx_point_batch_allocations_batch    ON point_batch_allocations (batch_id, occurred_on);
CREATE INDEX idx_point_batch_allocations_issuance ON point_batch_allocations (issuance_id);
