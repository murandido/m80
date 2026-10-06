-- Reverses 000081. Drops the read bookkeeping and the stored session.
--
-- What is lost is the ability to read the balance, not a figure: the last balance read stays in
-- accounts.real_balance_points. The one real cost is the session - dropping the refresh token
-- means every linked account has to log in on LATAM's page again.
DROP TABLE IF EXISTS latam_sync_state;
DROP TYPE IF EXISTS latam_sync_state_kind;
ALTER TABLE accounts DROP COLUMN IF EXISTS latam_refresh_token;
