DROP TABLE IF EXISTS accounts;

CREATE TABLE accounts (
    id                  UUID                     NOT NULL DEFAULT gen_random_uuid(),
    loyalty_program_id  UUID                     NOT NULL,
    login               VARCHAR                  NULL,
    password            VARCHAR                  NULL,
    holder_id           UUID                     NULL,
    administrator_id    UUID                     NULL,
    complete            BOOLEAN                  NOT NULL DEFAULT FALSE,
    created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT accounts_pkey PRIMARY KEY (id),
    CONSTRAINT accounts_loyalty_program_fk
        FOREIGN KEY (loyalty_program_id) REFERENCES loyalty_programs(id) ON DELETE RESTRICT,
    CONSTRAINT accounts_holder_fk
        FOREIGN KEY (holder_id) REFERENCES people(id) ON DELETE RESTRICT,
    CONSTRAINT accounts_administrator_fk
        FOREIGN KEY (administrator_id) REFERENCES people(id) ON DELETE RESTRICT
);
