CREATE TABLE sales (
    id           UUID                     NOT NULL DEFAULT gen_random_uuid(),
    issuance_id  UUID                     NOT NULL,
    client_id    UUID                     NULL,
    paid         BOOLEAN                  NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT sales_pkey PRIMARY KEY (id),
    CONSTRAINT sales_issuance_unique UNIQUE (issuance_id),
    CONSTRAINT sales_issuance_fk
        FOREIGN KEY (issuance_id) REFERENCES issuances(id) ON DELETE RESTRICT,
    CONSTRAINT sales_client_fk
        FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE SET NULL
);
