CREATE TABLE clients (
    id         UUID                     NOT NULL DEFAULT gen_random_uuid(),
    name       VARCHAR                  NOT NULL,
    phone      VARCHAR                  NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT clients_pkey PRIMARY KEY (id),
    CONSTRAINT clients_name_unique UNIQUE (name)
);
