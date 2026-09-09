-- Drops every acquisition ever recorded: what each purchase cost, who funded it and how
-- its profit was split. None of it is written down anywhere else, and the allocations that
-- reference it (migration 50) go first, so going back down means the costing has to be
-- rebuilt from the spreadsheets by hand.
DROP TABLE IF EXISTS point_batches;
