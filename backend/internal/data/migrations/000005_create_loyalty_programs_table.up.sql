DROP TABLE IF EXISTS loyalty_programs;

CREATE TABLE loyalty_programs (
    id                  UUID                     NOT NULL DEFAULT gen_random_uuid(),
    name                VARCHAR                  NOT NULL,
    code                VARCHAR                  NOT NULL,
    default_airline_id  UUID                     NOT NULL,
    created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT loyalty_programs_pkey PRIMARY KEY (id),
    CONSTRAINT loyalty_programs_code_unique UNIQUE (code),
    CONSTRAINT loyalty_programs_default_airline_fk
        FOREIGN KEY (default_airline_id) REFERENCES airlines(id) ON DELETE RESTRICT
);
