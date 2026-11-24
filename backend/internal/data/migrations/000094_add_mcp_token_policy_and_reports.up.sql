-- Lets an MCP token belong to an agent that works on its own (a daily routine) with a narrower
-- authority than its owner, and gives that agent somewhere to leave what it found.
--
-- Additive only: two nullable columns on a table that only the MCP reads, and a new table.

-- Which free-write tools the token may run directly. NULL means all of them - the behaviour
-- every token had before, and still the right one for a person's own token. An empty array
-- means none: every write the agent attempts becomes a proposal for a dev to approve.
ALTER TABLE mcp_tokens ADD COLUMN allowed_write_tools TEXT[] NULL;

-- Ceilings on what the token may write directly, as {"max_batch_points": n,
-- "max_batches_per_day": n}. A write past a ceiling becomes a proposal, not an error. NULL
-- means no ceiling.
ALTER TABLE mcp_tokens ADD COLUMN limits JSONB NULL;

-- One report per token per day: what a routine checked, what it found and what it did about
-- it. Writing again on the same day replaces the report, so a routine that is run twice does
-- not leave two versions of one day.
CREATE TABLE mcp_reports (
    id          UUID PRIMARY KEY,
    token_id    UUID NOT NULL REFERENCES mcp_tokens(id) ON DELETE CASCADE,
    report_date DATE NOT NULL,
    title       TEXT NOT NULL,
    summary     TEXT NOT NULL,
    findings    JSONB NULL,
    actions     JSONB NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT mcp_reports_one_per_day UNIQUE (token_id, report_date)
);

CREATE INDEX mcp_reports_date_idx ON mcp_reports (report_date DESC);
