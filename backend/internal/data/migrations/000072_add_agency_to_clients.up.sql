-- Every client belongs to one agency, and a panel user only sees the clients of the agencies
-- granted to them (user_agencies, 000071).
--
-- Nullable on purpose, for now: a client that bought through more than one agency cannot be
-- assigned by a rule, only by someone who knows the business. Those stay NULL, show up for
-- devs as "clientes sem agência", and NOT NULL comes in a later migration once that list is
-- empty in production - the same road accounts.holder_id took.
ALTER TABLE clients
    ADD COLUMN agency_id UUID NULL
        CONSTRAINT fk_clients_agency REFERENCES agencies (id) ON DELETE RESTRICT;

CREATE INDEX idx_clients_agency_id ON clients (agency_id);

-- The agency of a client's sales, when every sale went through the same one. Sales of
-- deleted issuances do not count: a deleted issuance is a typo being erased, not business.
WITH client_agencies AS (
    SELECT sales.client_id, MIN(issuances.agency_id::text)::uuid AS agency_id
    FROM sales
    JOIN issuances ON issuances.id = sales.issuance_id AND issuances.deleted_at IS NULL
    WHERE sales.client_id IS NOT NULL
    GROUP BY sales.client_id
    HAVING COUNT(DISTINCT issuances.agency_id) = 1
)
UPDATE clients
SET agency_id = client_agencies.agency_id
FROM client_agencies
WHERE clients.id = client_agencies.client_id;

-- A client with no sale at all goes to the main agency: nothing says otherwise, and that is
-- where a client registered without an agency would have been served.
UPDATE clients
SET agency_id = (SELECT id FROM agencies WHERE is_default LIMIT 1)
WHERE agency_id IS NULL
  AND NOT EXISTS (
      SELECT 1
      FROM sales
      JOIN issuances ON issuances.id = sales.issuance_id AND issuances.deleted_at IS NULL
      WHERE sales.client_id = clients.id
  );

-- A name is now unique within an agency: two agencies may each have their "João". The
-- clients still waiting for an agency keep the old, global rule among themselves, since a
-- NULL agency_id never collides in the composite index.
ALTER TABLE clients DROP CONSTRAINT clients_name_unique;
CREATE UNIQUE INDEX idx_clients_agency_name ON clients (agency_id, name);
CREATE UNIQUE INDEX idx_clients_name_without_agency ON clients (name) WHERE agency_id IS NULL;
