ALTER TABLE users
    DROP COLUMN IF EXISTS partner_client_name,
    DROP COLUMN IF EXISTS is_partner;
