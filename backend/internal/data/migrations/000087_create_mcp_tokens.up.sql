-- Personal tokens for the M80 MCP server (internal/mcp).
--
-- The login JWT is the only credential the API knew, and it is the wrong one to hand to a
-- tool: it lives seven days, cannot be revoked, and works on every route. An MCP token is
-- long-lived but revocable, is accepted only by the /mcp endpoint, and is stored as a hash so
-- a database dump never yields a usable credential. Additive only: a new table.
CREATE TABLE mcp_tokens (
    id           UUID PRIMARY KEY,

    -- Users are soft-deleted, so RESTRICT never fires in practice; it keeps a hard delete from
    -- silently orphaning a credential that still acts as someone.
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,

    -- What the token is for ("Claude Code notebook"), so a list of tokens can be read by a human.
    name         TEXT NOT NULL,

    -- sha256 of the token, hex encoded. The token itself is shown once, at creation, and never
    -- stored: it is random, so a plain digest is enough - there is nothing to brute force.
    token_hash   TEXT NOT NULL UNIQUE,

    last_used_at TIMESTAMPTZ NULL,

    -- Revoked rather than deleted, so a revoked token still answers "who was this".
    revoked_at   TIMESTAMPTZ NULL,

    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX mcp_tokens_user_idx ON mcp_tokens (user_id, created_at DESC);
