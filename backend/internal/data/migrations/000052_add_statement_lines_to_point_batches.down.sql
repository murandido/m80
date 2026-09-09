-- Loses the provenance of every batch recorded from a statement. The batches themselves
-- survive; what goes is the panel's ability to tell that a credit already became one of
-- them, which brings back the double-recording this column exists to prevent.
ALTER TABLE point_batches DROP COLUMN IF EXISTS statement_lines;
