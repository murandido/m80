DROP TABLE IF EXISTS account_nicknames;

CREATE TABLE account_nicknames (
    id                  UUID                     NOT NULL DEFAULT gen_random_uuid(),
    account_id          UUID                     NOT NULL,
    loyalty_program_id  UUID                     NOT NULL,
    nickname            VARCHAR                  NOT NULL,
    created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT account_nicknames_pkey PRIMARY KEY (id),
    CONSTRAINT account_nicknames_account_fk
        FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE,
    CONSTRAINT account_nicknames_loyalty_program_fk
        FOREIGN KEY (loyalty_program_id) REFERENCES loyalty_programs(id) ON DELETE RESTRICT
);

-- Mesmo nickname pode existir em programas diferentes; único dentro do mesmo programa
CREATE UNIQUE INDEX idx_account_nicknames_program_nickname
    ON account_nicknames (loyalty_program_id, nickname);
