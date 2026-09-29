-- Which agencies each panel user may see and act on. dev is never listed here: the role
-- itself means "every agency", so a dev keeps seeing an agency created after this runs
-- without anyone having to remember to grant it.
CREATE TABLE user_agencies (
    user_id    UUID                     NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    agency_id  UUID                     NOT NULL REFERENCES agencies (id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT user_agencies_pkey PRIMARY KEY (user_id, agency_id)
);

CREATE INDEX idx_user_agencies_agency_id ON user_agencies (agency_id);

-- Everyone who works in the panel today sees every agency, and an empty list now means
-- "no access". Granting the current staff every current agency keeps the deploy from
-- locking anybody out; narrowing it down is a dev's job afterwards.
INSERT INTO user_agencies (user_id, agency_id)
SELECT u.id, a.id
FROM users u
CROSS JOIN agencies a
WHERE u.role IN ('user', 'admin')
  AND u.deleted_at IS NULL;
