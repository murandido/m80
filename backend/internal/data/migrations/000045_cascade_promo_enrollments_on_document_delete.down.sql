-- Puts the delete guard back. Nothing to restore first: whatever the cascade removed is
-- already gone, and this only changes what happens to the next delete.
--
-- The 422 in DeleteLoyaltyDocument is what makes this readable to an operator, so if
-- this is rolled back the handler has to come back with it.
ALTER TABLE promo_enrollments
    DROP CONSTRAINT promo_enrollments_document_fk;

ALTER TABLE promo_enrollments
    ADD CONSTRAINT promo_enrollments_document_fk
        FOREIGN KEY (document_id) REFERENCES loyalty_documents(id) ON DELETE RESTRICT;
