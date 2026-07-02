DROP TABLE IF EXISTS people;

CREATE TABLE people (
    id               UUID                     NOT NULL DEFAULT gen_random_uuid(),
    name             VARCHAR                  NOT NULL,
    cpf              VARCHAR                  NULL,
    birth_date       DATE                     NULL,
    email            VARCHAR                  NULL,
    phone            VARCHAR                  NULL,
    zip_code         VARCHAR                  NULL,
    state            VARCHAR                  NULL,
    city             VARCHAR                  NULL,
    address          VARCHAR                  NULL,
    address_number   VARCHAR                  NULL,
    complement       VARCHAR                  NULL,
    neighborhood     VARCHAR                  NULL,
    complete         BOOLEAN                  NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT people_pkey PRIMARY KEY (id)
);

-- Partial index: múltiplos NULLs não violam a unicidade (apenas CPFs preenchidos são únicos)
CREATE UNIQUE INDEX idx_people_cpf ON people (cpf) WHERE cpf IS NOT NULL;
