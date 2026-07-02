CREATE TYPE passenger_type AS ENUM ('ADT', 'CHD', 'INF');

CREATE TABLE issuance_passengers (
    id           UUID                     NOT NULL DEFAULT gen_random_uuid(),
    issuance_id  UUID                     NOT NULL,
    first_name   VARCHAR                  NOT NULL,
    last_name    VARCHAR                  NOT NULL,
    type         passenger_type           NOT NULL DEFAULT 'ADT',
    document     VARCHAR                  NULL,
    position     INTEGER                  NOT NULL,
    created_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT issuance_passengers_pkey PRIMARY KEY (id),
    CONSTRAINT issuance_passengers_issuance_fk
        FOREIGN KEY (issuance_id) REFERENCES issuances(id) ON DELETE CASCADE
);

CREATE INDEX idx_issuance_passengers_issuance_id ON issuance_passengers (issuance_id);
