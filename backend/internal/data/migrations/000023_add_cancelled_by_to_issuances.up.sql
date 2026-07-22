-- Denormalizes who cancelled the issuance onto the row itself (the audit_logs table
-- already records it, but the list filter needs it queryable). ON DELETE RESTRICT mirrors
-- issuances_issuer_fk: a user who cancelled an issuance can't be hard-deleted out from
-- under it.
ALTER TABLE issuances
    ADD COLUMN cancelled_by_id UUID NULL,
    ADD CONSTRAINT issuances_cancelled_by_fk
        FOREIGN KEY (cancelled_by_id) REFERENCES users(id) ON DELETE RESTRICT;
