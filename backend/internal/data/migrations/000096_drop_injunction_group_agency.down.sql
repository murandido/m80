-- Brings the column back for the previous release, which requires it. The original values
-- are gone: each Tropa gets the agency of its first issuer mapping that has one, or the
-- default agency.
ALTER TABLE injunction_groups ADD COLUMN agency_id UUID NULL REFERENCES agencies(id) ON DELETE RESTRICT;

UPDATE injunction_groups g
SET agency_id = COALESCE(
    (SELECT (issuer ->> 'agency_id')::uuid
     FROM jsonb_array_elements(COALESCE(g.mappings -> 'issuers', '[]'::jsonb)) AS issuer
     WHERE issuer ->> 'agency_id' IN (SELECT id::text FROM agencies)
     LIMIT 1),
    (SELECT id FROM agencies WHERE is_default LIMIT 1)
);

ALTER TABLE injunction_groups ALTER COLUMN agency_id SET NOT NULL;
