-- Takes the administrator back off exactly the accounts the up migration filled in, and puts
-- their completeness back as it was.
UPDATE accounts a
   SET administrator_id = NULL,
       complete = b.previous_complete
  FROM administrator_unification_backfill b
 WHERE a.id = b.account_id;
DROP TABLE IF EXISTS administrator_unification_backfill;
