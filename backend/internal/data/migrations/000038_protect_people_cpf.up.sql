-- people.cpf was the last sensitive field still readable by anyone holding a database dump:
-- plain text, unique-indexed, and searched with ILIKE. Account passwords and passenger
-- documents have been encrypted at rest since they were introduced; this closes the same gap
-- on the client registry, which is the largest collection of CPFs the system holds.
--
-- The encryption itself happens in Go - AES-256-GCM under ENCRYPTION_KEY, the same hooks
-- Account and IssuancePassenger already use - because SQL has no access to the key. This
-- migration only prepares the schema for it; data.BackfillPersonCPFs rewrites the rows on the
-- next API boot, right after the migrations run.

-- A blind index over the CPF, so it can still be looked up and still refuse duplicates once
-- the column itself is ciphertext. Same HMAC and same key as account_beneficiaries.
-- (migration 37), so a person and a beneficiary holding the same document produce the same
-- fingerprint - which is what lets an account holder be recognised without a plaintext CPF.
ALTER TABLE people ADD COLUMN cpf_fingerprint VARCHAR NULL;

CREATE UNIQUE INDEX idx_people_cpf_fingerprint
    ON people (cpf_fingerprint)
    WHERE cpf_fingerprint IS NOT NULL;

-- The old uniqueness has to go, and not because it is redundant: AES-GCM uses a random nonce,
-- so the same CPF ciphers differently every time and a unique index over the ciphertext would
-- happily accept the same person twice. From here on the fingerprint above is what keeps a
-- CPF from being registered to two people.
--
-- Dropped after the replacement exists, so there is no window with no uniqueness at all.
DROP INDEX IF EXISTS idx_people_cpf;
