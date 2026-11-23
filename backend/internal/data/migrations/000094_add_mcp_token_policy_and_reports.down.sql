-- Reverses 000094. What is lost is the agents' daily reports and every token's write policy;
-- the tokens themselves stay, and go back to running every free write directly.
DROP TABLE IF EXISTS mcp_reports;
ALTER TABLE mcp_tokens DROP COLUMN IF EXISTS limits;
ALTER TABLE mcp_tokens DROP COLUMN IF EXISTS allowed_write_tools;
