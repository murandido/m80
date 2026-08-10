-- Adding a value to a Postgres enum is append-only: it does not rewrite the table and
-- does not need a window. IF NOT EXISTS keeps it idempotent in case the value was ever
-- added by hand on an environment before this migration ran there.
ALTER TYPE user_role ADD VALUE IF NOT EXISTS 'dev';
