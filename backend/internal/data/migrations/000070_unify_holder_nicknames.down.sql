-- Removes exactly the aliases the up migration inserted. An issuance the bot registered
-- through one of them since keeps its account; only its account_nickname_id goes to NULL
-- (ON DELETE SET NULL), which is the record of which alias the message used.
DELETE FROM account_nicknames
 WHERE id IN (SELECT row_id FROM nickname_unification_backfill WHERE table_name = 'account_nicknames');
DELETE FROM person_nicknames
 WHERE id IN (SELECT row_id FROM nickname_unification_backfill WHERE table_name = 'person_nicknames');
DROP TABLE IF EXISTS nickname_unification_backfill;
