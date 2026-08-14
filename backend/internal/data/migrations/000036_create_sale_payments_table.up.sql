-- What the client has actually handed over, one row per movement, so a sale can be settled
-- in parts. Until now sales.paid was the whole answer: a R$ 1.000 sale with R$ 400 received
-- could only be "open" (denying the 400) or "paid" (denying the 600), and the operators kept
-- the real figure in the spreadsheet the panel was supposed to replace.
CREATE TABLE sale_payments (
    id            UUID                     NOT NULL DEFAULT gen_random_uuid(),
    sale_id       UUID                     NOT NULL,
    -- Centavos, like every other amount in the schema. Signed on purpose: a refund to the
    -- client is a negative movement, which is what lets the balance of a cancelled-and-
    -- refunded sale come back to zero instead of needing a second concept.
    amount_cents  INTEGER                  NOT NULL,
    -- The day the money moved, which is what the operator knows. created_at below records
    -- when the line was typed - a different question, and sometimes a different day.
    paid_on       DATE                     NOT NULL,
    note          TEXT                     NULL,
    created_by_id UUID                     NOT NULL,
    created_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT sale_payments_pkey PRIMARY KEY (id),
    -- A movement of nothing is not a fact about the sale, and would only ever be a typo.
    CONSTRAINT sale_payments_amount_not_zero CHECK (amount_cents <> 0),
    -- Deleting an issuance already deletes its sale, so cascading here just keeps the
    -- ledger from outliving what it was a ledger of.
    CONSTRAINT sale_payments_sale_fk
        FOREIGN KEY (sale_id) REFERENCES sales(id) ON DELETE CASCADE,
    CONSTRAINT sale_payments_created_by_fk
        FOREIGN KEY (created_by_id) REFERENCES users(id) ON DELETE RESTRICT
);

CREATE INDEX idx_sale_payments_sale ON sale_payments (sale_id);

-- Every sale already marked paid carries a real fact - the money came in - with no amount
-- and no date attached. Without this backfill, the whole history reads as unpaid the moment
-- this ships.
--
-- The amount has to be the one the screen was showing, so this mirrors pointsValueCents in
-- internal/api/v1/handlers/sales.go: points * value / 1000, rounded UP to the centavo,
-- plus the taxes. CEIL, not ROUND - the export spreadsheet rounds up too, and the two are
-- not allowed to disagree by a cent on the same issuance.
--
-- paid_on is sales.updated_at: the closest thing to "when it was marked paid" that the old
-- shape ever stored. created_by_id is the issuer, for the same reason - nobody recorded who
-- ticked the box.
INSERT INTO sale_payments (sale_id, amount_cents, paid_on, note, created_by_id, created_at)
SELECT
    sales.id,
    CEIL(issuances.points::numeric * issuances.value_of_the_thousand / 1000)::integer
        + COALESCE(issuances.taxes, 0),
    (sales.updated_at AT TIME ZONE 'America/Sao_Paulo')::date,
    'Baixa registrada antes do controle de pagamentos parciais',
    issuances.issuer_id,
    sales.updated_at
FROM sales
JOIN issuances ON issuances.id = sales.issuance_id
WHERE sales.paid = TRUE
  -- A sale worth nothing has nothing to have been paid; inserting a zero would trip the
  -- check constraint above and take the whole migration - and the API boot - down with it.
  AND CEIL(issuances.points::numeric * issuances.value_of_the_thousand / 1000)::integer
        + COALESCE(issuances.taxes, 0) <> 0;
