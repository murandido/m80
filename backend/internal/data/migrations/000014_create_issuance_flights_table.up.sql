CREATE TYPE flight_direction AS ENUM ('outbound', 'inbound');

CREATE TABLE issuance_flights (
    id                     UUID                     NOT NULL DEFAULT gen_random_uuid(),
    issuance_id            UUID                     NOT NULL,
    direction              flight_direction          NOT NULL,
    leg_order              INTEGER                  NOT NULL,
    flight_number          VARCHAR                  NOT NULL,
    origin_iata_code       VARCHAR                  NULL,
    destination_iata_code  VARCHAR                  NULL,
    departure_at           TIMESTAMP WITH TIME ZONE NOT NULL,
    arrival_at             TIMESTAMP WITH TIME ZONE NOT NULL,
    duration_min           INTEGER                  NULL,
    cabin                  VARCHAR                  NULL,
    created_at             TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT issuance_flights_pkey PRIMARY KEY (id),
    CONSTRAINT issuance_flights_issuance_fk
        FOREIGN KEY (issuance_id) REFERENCES issuances(id) ON DELETE CASCADE,
    CONSTRAINT issuance_flights_origin_airport_fk
        FOREIGN KEY (origin_iata_code) REFERENCES airports(iata_code) ON DELETE RESTRICT,
    CONSTRAINT issuance_flights_destination_airport_fk
        FOREIGN KEY (destination_iata_code) REFERENCES airports(iata_code) ON DELETE RESTRICT
);

CREATE INDEX idx_issuance_flights_issuance_id ON issuance_flights (issuance_id);
