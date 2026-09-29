-- One administrator per holder: a live account with no administrator takes the one its holder's
-- other live accounts agree on. From here on the API and the bot keep it that way
-- (internal/account/administrators.go); this brings the older rows into line, once.
--
-- Only where there is nothing to decide. A holder whose accounts already name two different
-- administrators is left alone - the panel flags that row until an operator picks one.
--
-- An administrator is one of the fields an account needs to count as complete, so the flag is
-- recomputed the way Account.ComputeComplete does it (program and holder are already set on
-- every row touched). The previous value is recorded with each row, so the down migration puts
-- back exactly what was there.
CREATE TABLE administrator_unification_backfill (
    account_id        UUID    NOT NULL,
    previous_complete BOOLEAN NOT NULL
);

WITH single_administrator AS (
    SELECT holder_id, MIN(administrator_id::text)::uuid AS administrator_id
      FROM accounts
     WHERE deleted_at IS NULL AND holder_id IS NOT NULL AND administrator_id IS NOT NULL
     GROUP BY holder_id
    HAVING COUNT(DISTINCT administrator_id) = 1
),
targets AS (
    SELECT a.id, a.complete AS previous_complete, s.administrator_id
      FROM accounts a
      JOIN single_administrator s ON s.holder_id = a.holder_id
     WHERE a.deleted_at IS NULL AND a.administrator_id IS NULL
),
recorded AS (
    INSERT INTO administrator_unification_backfill (account_id, previous_complete)
    SELECT id, previous_complete FROM targets
    RETURNING account_id
)
UPDATE accounts a
   SET administrator_id = t.administrator_id,
       complete = a.complete OR (a.login IS NOT NULL AND a.password IS NOT NULL)
  FROM targets t
 WHERE a.id = t.id
   AND a.id IN (SELECT account_id FROM recorded);
