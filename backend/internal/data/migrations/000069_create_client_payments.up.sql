-- A lump sum a client handed the agency, before it is known which tickets it pays for.
--
-- Clients pay in round amounts, not per ticket. The payments screen takes the amount and
-- spreads it over the client's open sales, oldest first. The row below is the fact the
-- operator typed; the sale_payments it produced are derived from it and are rebuilt
-- whenever the picture changes (a payment undone, a sale moved to another client), which
-- is what keeps a typo from turning into money nobody can see or take back. What no open
-- sale absorbs is the client's credit - there is no balance column, it is always
-- "payments received minus what they paid for".
CREATE TABLE client_payments (
    id            UUID                     NOT NULL DEFAULT gen_random_uuid(),
    client_id     UUID                     NOT NULL,
    -- Centavos. Money handed back belongs to the sale it came from, not to a lump sum.
    amount_cents  INTEGER                  NOT NULL,
    paid_on       DATE                     NOT NULL,
    created_by_id UUID                     NOT NULL,
    created_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    -- Undoing a payment keeps the row: it is the history the operator reads to see what
    -- happened to a wrong amount. Only active rows are spread over sales.
    deleted_at    TIMESTAMP WITH TIME ZONE NULL,
    deleted_by_id UUID                     NULL,
    CONSTRAINT client_payments_pkey PRIMARY KEY (id),
    CONSTRAINT client_payments_amount_positive CHECK (amount_cents > 0),
    -- RESTRICT, not CASCADE: deleting the client would take the derived sale_payments with
    -- it and leave their sales marked paid for money that no longer exists anywhere.
    CONSTRAINT client_payments_client_fk
        FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE RESTRICT,
    CONSTRAINT client_payments_created_by_fk
        FOREIGN KEY (created_by_id) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT client_payments_deleted_by_fk
        FOREIGN KEY (deleted_by_id) REFERENCES users(id) ON DELETE RESTRICT
);

CREATE INDEX idx_client_payments_client ON client_payments (client_id);

-- The lump sum a sale payment was spread from. NULL is a payment recorded on the sale
-- itself (the Pago switch, the old per-sale statement), which the spreading never touches.
-- Nullable with no default, so adding it rewrites no row.
ALTER TABLE sale_payments ADD COLUMN client_payment_id UUID NULL
    CONSTRAINT sale_payments_client_payment_fk
        REFERENCES client_payments(id) ON DELETE RESTRICT;

CREATE INDEX idx_sale_payments_client_payment ON sale_payments (client_payment_id)
    WHERE client_payment_id IS NOT NULL;
