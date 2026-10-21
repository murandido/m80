-- What a sale received beyond what it is worth is the client's credit, not money to hand
-- back: the agency never refunds, it carries the difference to the client's next tickets.
-- A sale paid by the Pago switch and then made cheaper (taxes corrected, price typed lower,
-- an alteration, a cancellation) used to keep that surplus stuck on itself as "Devolver",
-- while another sale of the same client showed as still owing.
--
-- The surplus is moved by derived lines, rebuilt by the client replay like the lines of a
-- client payment: a negative one on the sale with the surplus, positive ones on the sales it
-- covers. This column names the sale the money came from, which is what marks a line as one
-- of those and lets the replay find and rebuild them.
--
-- Additive and nullable: every existing line was typed on its sale or spread from a client
-- payment, and stays so.
ALTER TABLE sale_payments ADD COLUMN credit_from_sale_id UUID NULL;

-- Deleting the sale the surplus came from takes its transfers with it, the same way its own
-- ledger goes (sale_payments_sale_fk).
ALTER TABLE sale_payments ADD CONSTRAINT sale_payments_credit_from_sale_fk
    FOREIGN KEY (credit_from_sale_id) REFERENCES sales(id) ON DELETE CASCADE;

CREATE INDEX idx_sale_payments_credit_from_sale ON sale_payments (credit_from_sale_id)
    WHERE credit_from_sale_id IS NOT NULL;
