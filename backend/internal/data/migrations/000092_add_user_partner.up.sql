-- A panel user is either part of the team or a partner who issues on their own behalf.
-- A partner's issuances are sold to the partner, so partner_client_name is the client
-- their sales are linked to (looked up, or created, in the issuance's agency).
--
-- Both columns are additive: is_partner defaults to false, so every existing user stays
-- on the team and no sale is touched by this migration.
ALTER TABLE users
    ADD COLUMN is_partner BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN partner_client_name TEXT;
