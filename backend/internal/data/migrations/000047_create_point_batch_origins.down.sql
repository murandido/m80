-- Safe to drop only while nothing references it; point_batches (migration 49) is what
-- makes this table load-bearing, and its own down runs first.
DROP TABLE IF EXISTS point_batch_origins;
