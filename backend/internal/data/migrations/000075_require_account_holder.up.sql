-- Every account has a holder: the panel shows an account as part of its holder's row, the API
-- has refused a holderless one since the accounts screen became one row per holder, and
-- production was checked read-only before this was written - no row, archived ones included,
-- lacked a holder.
--
-- If one appears anyway the ALTER fails and the deploy stops here: that is the point of making
-- the database say it, rather than the API alone.
ALTER TABLE accounts ALTER COLUMN holder_id SET NOT NULL;
