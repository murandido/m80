-- Dropping these loses which accounts were on the inventory regime and what each one's
-- default split was. Both are typed by hand and recorded nowhere else, so going back down
-- means re-marking every account. The batches themselves survive - they carry their own
-- frozen percent - so no costing history is lost with them.
ALTER TABLE accounts DROP COLUMN IF EXISTS default_profit_share_percent;
ALTER TABLE accounts DROP COLUMN IF EXISTS owns_point_inventory;
