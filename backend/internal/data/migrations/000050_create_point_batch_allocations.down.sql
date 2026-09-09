-- Safe: every row here is derived from the batches and the issuances, and a recompute
-- rebuilds all of it. Nothing typed by hand lives in this table.
DROP TABLE IF EXISTS point_batch_allocations;
