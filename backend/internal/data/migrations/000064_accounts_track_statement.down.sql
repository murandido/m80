-- Reverses 000064. Dropping the column returns the decision to owns_point_inventory.
--
-- What is lost is the distinction itself: an account that was tracking a statement without
-- owning its inventory stops being read, and its stored session and synced window become
-- unreachable (they are not deleted - migration 63 owns those). Turning the feature back on
-- means setting the flag again on those accounts.
ALTER TABLE accounts DROP COLUMN IF EXISTS tracks_statement;
