-- The Tropas: groups of injunction accounts that keep their issuances in a workbook on a
-- personal OneDrive, and the Microsoft login the API reads those workbooks with.
--
-- Additive only: two new tables, nothing existing changes.

-- The Microsoft account the workbooks are shared with. One per installation: the files are
-- shared with one person, and the API reads them as that person. Connecting again replaces
-- the row.
CREATE TABLE microsoft_connections (
    id              UUID PRIMARY KEY,
    -- Who clicked Connect in the panel. RESTRICT: users are soft-deleted, so it never fires;
    -- it keeps a hard delete from orphaning whose consent this is.
    connected_by_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    account_name    TEXT NULL,
    account_email   TEXT NULL,
    -- Encrypted at rest (utils.Encrypt). It rotates on every use, so it is rewritten often.
    refresh_token   TEXT NOT NULL,
    -- TEXT plus a CHECK rather than an ENUM: adding a value to a Postgres ENUM is a migration
    -- the down cannot undo.
    state           TEXT NOT NULL DEFAULT 'ok',
    detail          TEXT NULL,
    last_ok_at      TIMESTAMPTZ NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT microsoft_connections_state_known CHECK (state IN ('ok', 'needs_login', 'error'))
);

-- A Tropa: the injunction accounts one workbook covers, the WhatsApp group the agent talks
-- to about them, and how the workbook's labels map to the panel.
CREATE TABLE injunction_groups (
    id                 UUID PRIMARY KEY,
    name               TEXT NOT NULL,
    agency_id          UUID NOT NULL REFERENCES agencies(id) ON DELETE RESTRICT,
    -- The group's name as the WhatsApp MCP lists it. Free text: the agent looks it up there.
    whatsapp_chat_name TEXT NULL,
    -- The OneDrive sharing link. Encrypted at rest: it opens a workbook full of passenger
    -- names to anyone signed in with access.
    spreadsheet_url    TEXT NULL,
    -- {"accounts": [{"program_code", "account_label", "account_id"}],
    --  "issuers": [{"issuer_name", "user_id"}]}. Only rows whose account label is listed
    -- here are read: the same workbook also carries accounts that are not injunctions.
    mappings           JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX injunction_groups_name_key ON injunction_groups (lower(name));
