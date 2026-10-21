-- Reverses 000082. Every recorded alteration is lost with the table, so take a dump first
-- (make db-backup). The slices that priced them go too, and the next recompute of an open
-- period costs the issuances as if the tickets had never changed.
ALTER TABLE point_batch_allocations DROP COLUMN IF EXISTS adjustment_id;
DROP TABLE IF EXISTS issuance_adjustments;
