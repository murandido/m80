-- A client's credit with the agency: money received that no open sale was left to absorb.
--
-- The payments screen takes a lump sum from a client and spreads it over their open sales,
-- oldest first. Whatever is left over has no sale to sit on yet - it belongs to the client,
-- and is spent on the next sale linked to them. This is that balance, kept as a ledger
-- rather than a single number so it can always be explained: positive rows are surpluses
-- that came in, negative rows are credit spent on a sale. The balance is the sum.
CREATE TABLE client_credits (
    id            UUID                     NOT NULL DEFAULT gen_random_uuid(),
    client_id     UUID                     NOT NULL,
    -- Centavos, signed: + money kept for the client, - money spent on one of their sales.
    amount_cents  INTEGER                  NOT NULL,
    -- The sale the credit was spent on. Only set on negative rows; SET NULL so deleting an
    -- issuance (which deletes its sale) cannot take the history of the balance with it.
    sale_id       UUID                     NULL,
    note          TEXT                     NULL,
    created_by_id UUID                     NOT NULL,
    created_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT client_credits_pkey PRIMARY KEY (id),
    CONSTRAINT client_credits_amount_not_zero CHECK (amount_cents <> 0),
    -- Clients are hard-deleted, and sales.client_id already lets go of them on delete. A
    -- balance with no client has nobody left to spend it on, so it goes with the client.
    CONSTRAINT client_credits_client_fk
        FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE CASCADE,
    CONSTRAINT client_credits_sale_fk
        FOREIGN KEY (sale_id) REFERENCES sales(id) ON DELETE SET NULL,
    CONSTRAINT client_credits_created_by_fk
        FOREIGN KEY (created_by_id) REFERENCES users(id) ON DELETE RESTRICT
);

CREATE INDEX idx_client_credits_client ON client_credits (client_id);
