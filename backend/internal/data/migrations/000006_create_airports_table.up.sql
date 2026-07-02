DROP TABLE IF EXISTS airports;

CREATE TABLE airports (
    id         UUID                     NOT NULL DEFAULT gen_random_uuid(),
    iata_code  VARCHAR                  NOT NULL,
    name       VARCHAR                  NOT NULL,
    city       VARCHAR                  NULL,
    state      VARCHAR                  NULL,
    country    VARCHAR                  NOT NULL DEFAULT 'BR',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT airports_pkey PRIMARY KEY (id),
    CONSTRAINT airports_iata_code_unique UNIQUE (iata_code)
);
