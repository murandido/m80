-- Loyalty programs cap how many people an account may issue for in a period, and until now
-- that cap lived outside the system - in the head of whoever was issuing. Going over it does
-- not fail politely: the program blocks the account, and by then the money is already in.
--
-- Two things ship together here. The account's category, because part of the accounts operate
-- under a court injunction ("liminar") that suspends the beneficiary cap entirely, and a
-- third slice has unknown provenance where neither can be asserted. And the CPF control
-- itself, which only makes sense on the accounts that are actually capped.

CREATE TYPE account_category AS ENUM ('unidentified', 'standard', 'injunction');

-- Smiles counts distinct people per calendar year; LATAM Pass counts *every issuance* over a
-- rolling 12 months, so re-issuing for the same person consumes another slot there and does
-- not on Smiles. Two dimensions, not one, because they vary independently.
CREATE TYPE cpf_limit_window AS ENUM ('calendar_year', 'rolling_12_months');
CREATE TYPE cpf_limit_mode AS ENUM ('distinct_people', 'per_issuance');

-- 'unidentified' is the honest default for everything already in the table: nobody has
-- classified anything yet, and defaulting to 'standard' would switch on a fake limit for an
-- account that holds an injunction.
ALTER TABLE accounts ADD COLUMN category account_category NOT NULL DEFAULT 'unidentified';

-- Nullable on purpose: a program nobody has researched yet has no limit to enforce, which is
-- different from a limit of zero. The API renders no counter at all in that case.
ALTER TABLE loyalty_programs ADD COLUMN cpf_limit INTEGER NULL;
ALTER TABLE loyalty_programs ADD COLUMN cpf_limit_window cpf_limit_window NULL;
ALTER TABLE loyalty_programs ADD COLUMN cpf_limit_mode cpf_limit_mode NULL;

-- The people an account has issued for, owned by the account rather than by people: the
-- programs cap per account, and most of what we know about a passenger never becomes a
-- registered person. Deriving this from the issuances at read time is not possible -
-- issuance_passengers.document is encrypted with a random nonce (so no COUNT DISTINCT and no
-- index), passengers only exist for airlines Traveos syncs, and the spreadsheet import writes
-- nothing but issuances.main_passenger_name.
CREATE TABLE account_beneficiaries (
    id              UUID                     NOT NULL DEFAULT gen_random_uuid(),
    account_id      UUID                     NOT NULL,
    -- HMAC-SHA256 of the normalized CPF, never the CPF. Deterministic, so it works as a
    -- unique index and as the match key, which is all the resolution needs; and with the key
    -- held outside the database a leaked dump does not hand over anyone's document. A plain
    -- SHA-256 would not do: check digits leave ~10^9 valid CPFs, brute-forceable in minutes.
    cpf_fingerprint VARCHAR                  NULL,
    -- What the screen shows, e.g. '***.456.789-**'. Enough for a human to tell two
    -- beneficiaries apart, not enough to reconstruct the number.
    cpf_masked      VARCHAR                  NULL,
    name            VARCHAR                  NOT NULL,
    -- Accent-free, uppercased, single-spaced: the fallback match key when no CPF ever
    -- arrived, which is the common case for imported history.
    -- Must stay byte-identical to utils.NormalizeName: the backfill below writes this
    -- column in SQL and the API matches against it in Go. If the two ever disagree, the
    -- first issuance after a deploy creates a duplicate of somebody already in the table.
    normalized_name VARCHAR                  NOT NULL,
    -- None of the three programs counts the account holder against the cap.
    is_holder       BOOLEAN                  NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT account_beneficiaries_pkey PRIMARY KEY (id),
    CONSTRAINT account_beneficiaries_account_fk
        FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
);

-- Two partial uniques rather than one: a beneficiary we know by CPF is identified by it, and
-- one we only know by name is identified by the name. A row that later learns its CPF moves
-- from the second index to the first.
CREATE UNIQUE INDEX idx_account_beneficiaries_cpf
    ON account_beneficiaries (account_id, cpf_fingerprint)
    WHERE cpf_fingerprint IS NOT NULL;
CREATE UNIQUE INDEX idx_account_beneficiaries_name
    ON account_beneficiaries (account_id, normalized_name)
    WHERE cpf_fingerprint IS NULL;

-- One row per issuance that consumed a beneficiary. The consumption is a window over this
-- history, never a stored counter: on 1 January the Smiles number falls to zero by itself,
-- the LATAM one slides day by day, and "how many in 2026" stays answerable in 2027. A
-- last_used_at on the beneficiary could not do that - the second use overwrites the first.
--
-- The same rows answer both counting modes: COUNT(DISTINCT beneficiary_id) for Smiles,
-- COUNT(*) for LATAM.
CREATE TABLE account_beneficiary_uses (
    id              UUID                     NOT NULL DEFAULT gen_random_uuid(),
    beneficiary_id  UUID                     NOT NULL,
    issuance_id     UUID                     NULL,
    used_at         TIMESTAMP WITH TIME ZONE NOT NULL,
    -- 'issuance' when the pipeline recorded it, 'manual' when an operator did.
    source          VARCHAR                  NOT NULL DEFAULT 'issuance',
    created_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT account_beneficiary_uses_pkey PRIMARY KEY (id),
    CONSTRAINT account_beneficiary_uses_beneficiary_fk
        FOREIGN KEY (beneficiary_id) REFERENCES account_beneficiaries(id) ON DELETE CASCADE,
    -- SET NULL, not CASCADE: deleting the issuance does not give the slot back, for the same
    -- reason cancelling it does not. The programs charge the slot the moment it is used.
    CONSTRAINT account_beneficiary_uses_issuance_fk
        FOREIGN KEY (issuance_id) REFERENCES issuances(id) ON DELETE SET NULL
);

CREATE INDEX idx_account_beneficiary_uses_window
    ON account_beneficiary_uses (beneficiary_id, used_at);
-- Traveos sync deletes and reinserts an issuance's passengers, so registering the same
-- passenger twice must not consume the slot twice. This is what makes Register idempotent.
--
-- Not a partial index, even though it only ever constrains rows with an issuance: Postgres
-- treats NULLs as distinct, so manual uses (issuance_id NULL) are unconstrained anyway, and
-- a full index is the one an ON CONFLICT target can infer.
CREATE UNIQUE INDEX idx_account_beneficiary_uses_issuance
    ON account_beneficiary_uses (beneficiary_id, issuance_id);

-- Seed from what the issuances already say, so the screen opens with a real number instead
-- of zero everywhere.
--
-- The passenger rows come first where they exist: an issuance synced from Traveos knows the
-- whole booking, and one of them here carries eight people on a single ticket. Rebuilding
-- from main_passenger_name alone would count that as one person and understate the account
-- by seven. Only issuances with no passengers at all fall back to that one name - which is
-- most of them, since the spreadsheet import and the bot write nothing else.
--
-- This has to agree with account.Reconcile, which answers the same question in Go: the
-- operator can press "Reconstruir pelas emissões" at any time, and the number must not jump.
--
-- No fingerprint: the documents are encrypted with a random nonce, which is exactly the
-- "match by name" case this design accounts for.
CREATE TEMP TABLE issuance_people AS
SELECT
    issuances.id         AS issuance_id,
    issuances.account_id AS account_id,
    issuances.created_at AS used_at,
    UPPER(UNACCENT(REGEXP_REPLACE(
        TRIM(issuance_passengers.first_name || ' ' || issuance_passengers.last_name),
        '\s+', ' ', 'g'))) AS normalized_name,
    TRIM(issuance_passengers.first_name || ' ' || issuance_passengers.last_name) AS name
FROM issuances
JOIN issuance_passengers ON issuance_passengers.issuance_id = issuances.id
UNION ALL
SELECT
    issuances.id,
    issuances.account_id,
    issuances.created_at,
    UPPER(UNACCENT(REGEXP_REPLACE(TRIM(issuances.main_passenger_name), '\s+', ' ', 'g'))),
    TRIM(issuances.main_passenger_name)
FROM issuances
WHERE NOT EXISTS (
        SELECT 1 FROM issuance_passengers
        WHERE issuance_passengers.issuance_id = issuances.id
      )
  AND issuances.main_passenger_name IS NOT NULL
  AND TRIM(issuances.main_passenger_name) <> '';

DELETE FROM issuance_people WHERE normalized_name = '';

INSERT INTO account_beneficiaries (account_id, name, normalized_name)
SELECT
    account_id,
    -- An arbitrary spelling among the ones that collapsed into this key; the operator can
    -- fix it in the panel. Only the normalized form matters for matching.
    MIN(name),
    normalized_name
FROM issuance_people
GROUP BY account_id, normalized_name;

-- One use per (person, issuance). DISTINCT because the same passenger can appear twice on a
-- ticket the vendor listed oddly, and the unique index would refuse the second row.
-- Cancelled and soft-deleted issuances are included on purpose: a used slot stays used.
INSERT INTO account_beneficiary_uses (beneficiary_id, issuance_id, used_at, source)
SELECT DISTINCT
    account_beneficiaries.id,
    issuance_people.issuance_id,
    issuance_people.used_at,
    'issuance'
FROM issuance_people
JOIN account_beneficiaries
    ON account_beneficiaries.account_id = issuance_people.account_id
   AND account_beneficiaries.normalized_name = issuance_people.normalized_name;

DROP TABLE issuance_people;

-- Mark the account holders so they stay out of the count, by name: this runs before
-- migration 38 gives people a fingerprint, so there is nothing stronger to match on here.
-- From 38 onwards account.Register compares fingerprints as well.
UPDATE account_beneficiaries
SET is_holder = TRUE
FROM accounts
JOIN people ON people.id = accounts.holder_id
WHERE accounts.id = account_beneficiaries.account_id
  AND account_beneficiaries.normalized_name = UPPER(UNACCENT(REGEXP_REPLACE(TRIM(people.name), '\s+', ' ', 'g')));

-- The published rules as of today, for the programs already in this database. New
-- environments get the same values from seeds/loyalty_programs.json instead, since the
-- seeder runs after the migrations and creates the programs from scratch there.
--
-- Matched on the aliases too: an operator may have registered the program as "TudoAzul" or
-- "LATAM Pass" long before the seeder existed.
UPDATE loyalty_programs SET cpf_limit = 25, cpf_limit_window = 'calendar_year', cpf_limit_mode = 'distinct_people'
    WHERE UPPER(code) = 'SMILES' OR UPPER(name) = 'SMILES';
UPDATE loyalty_programs SET cpf_limit = 24, cpf_limit_window = 'rolling_12_months', cpf_limit_mode = 'per_issuance'
    WHERE UPPER(code) IN ('LATAM', 'LATAMPASS') OR UPPER(name) IN ('LATAM PASS', 'LATAMPASS', 'LATAM');
UPDATE loyalty_programs SET cpf_limit = 5, cpf_limit_window = 'calendar_year', cpf_limit_mode = 'distinct_people'
    WHERE UPPER(code) IN ('AZUL', 'TUDOAZUL') OR UPPER(name) IN ('AZUL', 'TUDOAZUL', 'TUDO AZUL');
