DROP TABLE IF EXISTS agencies;

CREATE TABLE agencies (
    id         UUID                     NOT NULL DEFAULT gen_random_uuid(),
    name       VARCHAR                  NOT NULL,
    is_default BOOLEAN                  NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT agencies_pkey PRIMARY KEY (id)
);

-- Garante no máximo uma agência com is_default = true simultaneamente
CREATE UNIQUE INDEX idx_agencies_is_default ON agencies (is_default) WHERE is_default = TRUE;
