-- Which loyalty accounts a statement_client user may read the statement of. Only that role
-- has rows here: staff reach accounts through their role, not through grants, so this table
-- never narrows what the team sees.
CREATE TABLE user_accounts (
    user_id    UUID                     NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    account_id UUID                     NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT user_accounts_pkey PRIMARY KEY (user_id, account_id)
);

CREATE INDEX idx_user_accounts_account_id ON user_accounts (account_id);
