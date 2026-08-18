-- ⚠️ This does NOT decrypt people.cpf, and it cannot: the key lives in ENCRYPTION_KEY, which
-- SQL has no access to. After running this the column keeps whatever the API wrote - "enc:..."
-- for every row the backfill reached.
--
-- Nothing is lost by that. utils.Decrypt passes plaintext through untouched and decrypts the
-- prefixed values, so rolling the API image *forward* again renders the CPFs correctly. What
-- an older image does - one that predates the hooks on Person - is show the ciphertext on
-- screen until it is rolled forward. That is the known price of encrypting in place, and it is
-- a display regression, not data loss.
--
-- Recreating the plaintext unique index works either way: ciphertext values are distinct by
-- construction, so it cannot fail on already-encrypted data. It just stops being meaningful.
CREATE UNIQUE INDEX IF NOT EXISTS idx_people_cpf ON people (cpf) WHERE cpf IS NOT NULL;

DROP INDEX IF EXISTS idx_people_cpf_fingerprint;
ALTER TABLE people DROP COLUMN IF EXISTS cpf_fingerprint;
