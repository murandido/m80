-- Dropping this unfreezes every account: with no closed period to stop at, the next
-- recompute rebuilds months that were already settled and paid. The allocations survive
-- the drop, but the first recompute after it overwrites them.
--
-- So going back down means re-closing each account's settled months before anything
-- recomputes, not just re-running this migration's up.
DROP TABLE IF EXISTS inventory_periods;
