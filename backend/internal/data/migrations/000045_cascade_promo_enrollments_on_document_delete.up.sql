-- Lets a loyalty document be deleted, taking its enrolment history with it.
--
-- The RESTRICT this replaces existed to keep the record of what was submitted to the
-- airlines' sites from being orphaned, and the delete handler answered 422 telling the
-- operator to deactivate instead. The cost was that a document which had ever enrolled
-- could never be removed - a CPF typed wrong stayed wrong forever, because the only way
-- to fix a document's CPF is to recreate it.
--
-- The history is now treated as disposable next to that. Deleting a CPF deletes the
-- proof of what was submitted for it, and only the daily dump brings that back.
ALTER TABLE promo_enrollments
    DROP CONSTRAINT promo_enrollments_document_fk;

ALTER TABLE promo_enrollments
    ADD CONSTRAINT promo_enrollments_document_fk
        FOREIGN KEY (document_id) REFERENCES loyalty_documents(id) ON DELETE CASCADE;
