-- A batch is one acquisition of points into one account: a Livelo transfer, a bank
-- campaign, a direct purchase from the airline, a month of club subscription.
--
-- It exists because the cost of a point is a property of the purchase, not of the booking
-- that later spends it. The spreadsheet already worked this way - its right-hand block
-- divided "CUSTO (R$)" by "QNT. PONTOS (COMPRA)" to get a cost per thousand, and every
-- issuance of the month was priced against whichever purchase it consumed.
--
-- One account, never two. accounts.loyalty_program_id is NOT NULL, so an account is
-- already of a single programme, and a Livelo transfer sent to Azul is a different batch
-- from one sent to Smiles even when both were bought in the same operation. The two
-- stocks never mix, which is exactly what the spreadsheet's separate AZUL and SMILES tabs
-- were saying.
CREATE TABLE point_batches (
    id                   UUID     NOT NULL DEFAULT gen_random_uuid(),
    account_id           UUID     NOT NULL,
    -- NULL only on an opening balance, which is a carried-forward remainder rather than a
    -- purchase from anybody. See the CHECK below.
    origin_id            UUID     NULL,
    -- The date the points landed, and the only thing that orders the FIFO queue. Points
    -- returned to a batch by a refund rejoin the queue here, at the batch's own date -
    -- they are the same points as always, so they leave again before anything newer.
    acquired_on          DATE     NOT NULL,
    points               INTEGER  NOT NULL,
    -- Zero is a real, common value, not a missing one: a club subscription and flight
    -- accrual both put points in the account at no cost, and the spreadsheet priced them
    -- at a cost per thousand of zero so the whole sale counted as profit.
    cost_cents           INTEGER  NOT NULL,
    -- Which agency paid for the batch. This is what makes a settlement necessary at all:
    -- there is something to settle only when the agency that issued is not this one, so
    -- an account whose bookings all belong to its funder nets to zero on its own, with
    -- nothing to configure.
    funding_agency_id    UUID     NOT NULL,
    -- Frozen at creation from accounts.default_profit_share_percent. Read live from the
    -- account instead, the day the arrangement changes from 50 to 60 every past month
    -- would silently change with it - including months already settled and paid.
    profit_share_percent SMALLINT NOT NULL,
    opening              BOOLEAN  NOT NULL DEFAULT FALSE,
    -- The spreadsheet's "ACERTADO" column, as a timestamp instead of SIM/NÃO so the
    -- settlement can be dated.
    settled_at           TIMESTAMP WITH TIME ZONE NULL,
    note                 TEXT     NULL,
    created_at           TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    -- Archiving, the same shape accounts uses (migration 29): a batch that ever priced an
    -- issuance is referenced ON DELETE RESTRICT by point_batch_allocations, so a real
    -- delete on it is refused by Postgres forever. Soft-deleting is what lets a batch
    -- entered by mistake leave the screen without erasing what it costed.
    deleted_at           TIMESTAMP WITH TIME ZONE NULL,
    CONSTRAINT point_batches_pkey PRIMARY KEY (id),
    CONSTRAINT point_batches_account_fk
        FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE RESTRICT,
    CONSTRAINT point_batches_origin_fk
        FOREIGN KEY (origin_id) REFERENCES point_batch_origins(id) ON DELETE RESTRICT,
    CONSTRAINT point_batches_funding_agency_fk
        FOREIGN KEY (funding_agency_id) REFERENCES agencies(id) ON DELETE RESTRICT,
    -- An ordinary batch came from somewhere; only a carried-forward opening balance did not.
    CONSTRAINT point_batches_origin_required
        CHECK (origin_id IS NOT NULL OR opening),
    CONSTRAINT point_batches_points_positive     CHECK (points > 0),
    CONSTRAINT point_batches_cost_not_negative   CHECK (cost_cents >= 0),
    CONSTRAINT point_batches_share_is_a_percent  CHECK (profit_share_percent BETWEEN 0 AND 100)
);

-- The FIFO read: every batch of one account, oldest first. Partial because an archived
-- batch must never be consumed, and this way it is not even in the index.
CREATE INDEX idx_point_batches_fifo
    ON point_batches (account_id, acquired_on)
    WHERE deleted_at IS NULL;
