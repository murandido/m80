-- Restores the idempotency guard.
--
-- This fails if duplicate successes were recorded while the index was gone, and that is
-- deliberate: there is no way to restore the invariant except by deleting history, and
-- doing that silently inside a rollback would be worse than stopping. Run this first,
-- having decided that losing the repeats is what you want:
--
--   DELETE FROM promo_enrollments a
--    USING promo_enrollments b
--    WHERE a.status = 'success' AND b.status = 'success'
--      AND a.url_key = b.url_key AND a.document_id = b.document_id
--      AND a.created_at > b.created_at;
CREATE UNIQUE INDEX idx_promo_enrollments_unique_success
    ON promo_enrollments (url_key, document_id) WHERE status = 'success';
