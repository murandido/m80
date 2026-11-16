-- The personal MCP server (internal/mcp): the tokens that open it, the trail of every tool call
-- it served, and the proposals it raised for the operations a human has to approve.
--
-- Additive only: three new tables, nothing existing changes. Rolling back the image leaves them
-- idle.
--
-- IF NOT EXISTS throughout: this was first written as 000090, and a development database that
-- ran it under that number already has the tables while main's own 000090 took the number. The
-- guard lets such a database move on instead of failing at boot; a fresh one sees no difference.

-- A token is the only credential /mcp accepts, and it is accepted nowhere else: no /api/v1 route
-- reads this table. Only the sha256 of the token is stored; the plain value is shown once, when
-- it is created.
CREATE TABLE IF NOT EXISTS mcp_tokens (
    id           UUID PRIMARY KEY,

    -- Users are soft-deleted, so RESTRICT never fires in practice; it keeps a hard delete from
    -- orphaning whose authority a token carries.
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    last_used_at TIMESTAMPTZ NULL,
    revoked_at   TIMESTAMPTZ NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS mcp_tokens_user_idx ON mcp_tokens (user_id);

-- One row per tool call. The MCP writes some things without asking (creating and editing
-- issuances, recording point batches), and audit_logs neither covers anything but issuances nor
-- records edits at all - this is where those writes can be traced back to the agent.
CREATE TABLE IF NOT EXISTS mcp_calls (
    id          UUID PRIMARY KEY,
    -- CASCADE: a token is only ever revoked, never deleted, by the panel; a hard delete by hand
    -- is a deliberate wipe of its trail too.
    token_id    UUID NOT NULL REFERENCES mcp_tokens(id) ON DELETE CASCADE,
    tool        TEXT NOT NULL,
    args        JSONB NULL,
    ok          BOOLEAN NOT NULL,
    error       TEXT NULL,
    duration_ms INTEGER NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS mcp_calls_token_idx ON mcp_calls (token_id, created_at DESC);
CREATE INDEX IF NOT EXISTS mcp_calls_created_idx ON mcp_calls (created_at DESC);

-- An operation the agent may not run on its own (cancelling, deleting, closing a period). The
-- tool only records it; a dev approves it in the panel, and the operation then runs as the
-- approver, through the same /api/v1 route the panel itself would call.
CREATE TABLE IF NOT EXISTS mcp_proposals (
    id            UUID PRIMARY KEY,
    -- Whose token raised it.
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    tool          TEXT NOT NULL,
    summary       TEXT NOT NULL,
    -- [{method, path, body}] against /api/v1, run in order on approval.
    operations    JSONB NOT NULL,
    -- The state of the target when the proposal was made, so the reviewer sees what changes.
    preview       JSONB NULL,
    -- TEXT plus a CHECK rather than an ENUM: adding a value to a Postgres ENUM is a migration
    -- the down cannot undo.
    status        TEXT NOT NULL DEFAULT 'pending',
    result        JSONB NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at    TIMESTAMPTZ NULL,
    decided_by_id UUID NULL REFERENCES users(id) ON DELETE RESTRICT,

    CONSTRAINT mcp_proposals_status_known CHECK (status IN ('pending', 'applied', 'rejected', 'failed', 'expired'))
);

CREATE INDEX IF NOT EXISTS mcp_proposals_status_idx ON mcp_proposals (status, created_at DESC);
