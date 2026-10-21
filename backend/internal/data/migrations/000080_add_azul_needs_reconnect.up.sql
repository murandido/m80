-- A customerKey the Azul API stopped recognising (points/resume answers 404 - in practice the
-- holder changed the password and Azul issued a new key) is not a failure to retry: no number of
-- read rounds fixes it, only a fresh login does. It gets its own state so the ticker can stop
-- polling the account and the panel can ask for exactly that, instead of the account piling up
-- failures under the generic needs_attention (one did 717 rounds in six days before this).
--
-- Adding a value to a Postgres enum is append-only: it does not rewrite the table and does not
-- need a window. IF NOT EXISTS keeps it idempotent, like migration 32.
ALTER TYPE azul_sync_state_kind ADD VALUE IF NOT EXISTS 'needs_reconnect';
