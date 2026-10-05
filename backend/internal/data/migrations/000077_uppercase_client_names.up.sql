-- Client names are stored in upper case from now on (the API normalizes every create and
-- update), so the ones already registered are brought to the same spelling here.
--
-- A name whose upper-case form already belongs to another client of the same agency is left
-- alone on purpose: updating it would break idx_clients_agency_name (or the index for clients
-- without an agency), the migration would fail half-way and leave schema_migrations dirty,
-- and the API would refuse to boot. Those pairs are the same person registered twice, and are
-- settled on the clients screen - renamed or merged - by someone who knows which one is right.
-- Both rows of such a pair are skipped, not just one: which of them to keep is that decision.
UPDATE clients AS c
SET name = UPPER(c.name)
WHERE c.name <> UPPER(c.name)
  AND NOT EXISTS (
      SELECT 1
      FROM clients AS other
      WHERE other.id <> c.id
        AND other.agency_id IS NOT DISTINCT FROM c.agency_id
        AND UPPER(other.name) = UPPER(c.name)
  );
