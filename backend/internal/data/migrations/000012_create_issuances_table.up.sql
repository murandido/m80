DROP TABLE IF EXISTS issuances;
DROP TYPE IF EXISTS trip_type;

CREATE TYPE trip_type AS ENUM ('one_way', 'round_trip', 'multi_leg');

CREATE TABLE issuances (
    id                     UUID                     NOT NULL DEFAULT gen_random_uuid(),
    loyalty_program_id     UUID                     NOT NULL,
    airline_id             UUID                     NOT NULL,
    program_locator        VARCHAR                  NOT NULL,
    airline_locator        VARCHAR                  NULL,
    latam_purchase_number  VARCHAR                  NULL,
    main_passenger_name    VARCHAR                  NULL,
    trip_type              trip_type                NULL,
    origin_iata_code       VARCHAR                  NULL,
    destination_iata_code  VARCHAR                  NULL,
    points                 INTEGER                  NOT NULL,
    value_of_the_thousand  INTEGER                  NOT NULL,
    taxes                  INTEGER                  NULL,
    account_id             UUID                     NOT NULL,
    issuer_id              UUID                     NOT NULL,
    agency_id              UUID                     NOT NULL,
    complete               BOOLEAN                  NOT NULL DEFAULT FALSE,
    additional_services    JSONB                    NULL,
    observation            TEXT                     NULL,
    active                 BOOLEAN                  NOT NULL DEFAULT TRUE,
    cancelled_at           TIMESTAMP WITH TIME ZONE NULL,
    refunded_points        INTEGER                  NULL,
    refunded_taxes         INTEGER                  NULL,
    cancellation_fee       INTEGER                  NULL,
    created_at             TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at             TIMESTAMP WITH TIME ZONE NULL,
    CONSTRAINT issuances_pkey PRIMARY KEY (id),
    CONSTRAINT issuances_loyalty_program_fk
        FOREIGN KEY (loyalty_program_id) REFERENCES loyalty_programs(id) ON DELETE RESTRICT,
    CONSTRAINT issuances_airline_fk
        FOREIGN KEY (airline_id) REFERENCES airlines(id) ON DELETE RESTRICT,
    CONSTRAINT issuances_origin_airport_fk
        FOREIGN KEY (origin_iata_code) REFERENCES airports(iata_code) ON DELETE RESTRICT,
    CONSTRAINT issuances_destination_airport_fk
        FOREIGN KEY (destination_iata_code) REFERENCES airports(iata_code) ON DELETE RESTRICT,
    CONSTRAINT issuances_account_fk
        FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE RESTRICT,
    CONSTRAINT issuances_issuer_fk
        FOREIGN KEY (issuer_id) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT issuances_agency_fk
        FOREIGN KEY (agency_id) REFERENCES agencies(id) ON DELETE RESTRICT
);

CREATE INDEX idx_issuances_deleted_at ON issuances (deleted_at);

CREATE UNIQUE INDEX idx_issuances_program_locator_active
    ON issuances (program_locator)
    WHERE active = TRUE AND deleted_at IS NULL;
