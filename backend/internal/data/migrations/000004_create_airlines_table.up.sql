DROP TABLE IF EXISTS airlines;

CREATE TABLE airlines (
    id                       UUID                     NOT NULL DEFAULT gen_random_uuid(),
    name                     VARCHAR                  NOT NULL,
    code                     VARCHAR                  NOT NULL,
    traveos_supported        BOOLEAN                  NOT NULL DEFAULT FALSE,
    requires_last_name       BOOLEAN                  NOT NULL DEFAULT FALSE,
    requires_purchase_number BOOLEAN                  NOT NULL DEFAULT FALSE,
    created_at               TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT airlines_pkey PRIMARY KEY (id),
    CONSTRAINT airlines_code_unique UNIQUE (code)
);
