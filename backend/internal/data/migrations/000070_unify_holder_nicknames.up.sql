-- One list of aliases per holder: every alias of a person becomes an alias of each live
-- account they hold, and every alias of those accounts becomes the person's. From here on
-- the API and the bot keep it that way (internal/account/nicknames.go); this brings the rows
-- written before that into line, once.
--
-- Insert-only, and cautious about the two uniqueness rules:
--   * person_nicknames is unique across everybody, so an alias another person already has is
--     left alone, and so is one that two different holders would both claim;
--   * account_nicknames is unique within a program, so an alias another account of the same
--     program already has is left alone, and so is one two accounts would both claim.
-- Matching is case-insensitive, the way the bot matches; the spelling written first wins.
--
-- Every row inserted is recorded below, so the down migration removes exactly those.
CREATE TABLE nickname_unification_backfill (
    table_name TEXT NOT NULL,
    row_id     UUID NOT NULL
);

-- Person side.
WITH names AS (
    SELECT pn.person_id AS holder_id, pn.nickname, pn.created_at
      FROM person_nicknames pn
     WHERE EXISTS (SELECT 1 FROM accounts a WHERE a.holder_id = pn.person_id AND a.deleted_at IS NULL)
    UNION ALL
    SELECT a.holder_id, an.nickname, an.created_at
      FROM account_nicknames an
      JOIN accounts a ON a.id = an.account_id
     WHERE an.deleted_at IS NULL AND a.deleted_at IS NULL AND a.holder_id IS NOT NULL
),
spelled AS (
    SELECT DISTINCT ON (holder_id, LOWER(nickname)) holder_id, nickname
      FROM names
     ORDER BY holder_id, LOWER(nickname), created_at
),
candidates AS (
    SELECT s.holder_id, s.nickname
      FROM spelled s
     WHERE NOT EXISTS (SELECT 1 FROM person_nicknames pn WHERE LOWER(pn.nickname) = LOWER(s.nickname))
),
unclaimed AS (
    SELECT c.holder_id, c.nickname
      FROM candidates c
     WHERE (SELECT COUNT(DISTINCT o.holder_id) FROM candidates o WHERE LOWER(o.nickname) = LOWER(c.nickname)) = 1
),
inserted AS (
    INSERT INTO person_nicknames (person_id, nickname)
    SELECT holder_id, nickname FROM unclaimed
    RETURNING id
)
INSERT INTO nickname_unification_backfill (table_name, row_id)
SELECT 'person_nicknames', id FROM inserted;

-- Account side.
WITH names AS (
    SELECT pn.person_id AS holder_id, pn.nickname, pn.created_at
      FROM person_nicknames pn
    UNION ALL
    SELECT a.holder_id, an.nickname, an.created_at
      FROM account_nicknames an
      JOIN accounts a ON a.id = an.account_id
     WHERE an.deleted_at IS NULL AND a.deleted_at IS NULL AND a.holder_id IS NOT NULL
),
spelled AS (
    SELECT DISTINCT ON (holder_id, LOWER(nickname)) holder_id, nickname
      FROM names
     ORDER BY holder_id, LOWER(nickname), created_at
),
candidates AS (
    SELECT a.id AS account_id, a.loyalty_program_id, s.nickname
      FROM spelled s
      JOIN accounts a ON a.holder_id = s.holder_id AND a.deleted_at IS NULL
     WHERE NOT EXISTS (
         SELECT 1 FROM account_nicknames an
          WHERE an.loyalty_program_id = a.loyalty_program_id
            AND an.deleted_at IS NULL
            AND LOWER(an.nickname) = LOWER(s.nickname)
     )
),
unclaimed AS (
    SELECT c.account_id, c.loyalty_program_id, c.nickname
      FROM candidates c
     WHERE (SELECT COUNT(DISTINCT o.account_id) FROM candidates o
             WHERE o.loyalty_program_id = c.loyalty_program_id
               AND LOWER(o.nickname) = LOWER(c.nickname)) = 1
),
inserted AS (
    INSERT INTO account_nicknames (account_id, loyalty_program_id, nickname)
    SELECT account_id, loyalty_program_id, nickname FROM unclaimed
    RETURNING id
)
INSERT INTO nickname_unification_backfill (table_name, row_id)
SELECT 'account_nicknames', id FROM inserted;
