CREATE TABLE issuance_passenger_flights (
    id           UUID    NOT NULL DEFAULT gen_random_uuid(),
    passenger_id UUID    NOT NULL,
    flight_id    UUID    NOT NULL,
    seat         VARCHAR NULL,
    baggage_qty  INTEGER NULL DEFAULT 0,
    CONSTRAINT issuance_passenger_flights_pkey PRIMARY KEY (id),
    CONSTRAINT issuance_passenger_flights_passenger_fk
        FOREIGN KEY (passenger_id) REFERENCES issuance_passengers(id) ON DELETE CASCADE,
    CONSTRAINT issuance_passenger_flights_flight_fk
        FOREIGN KEY (flight_id) REFERENCES issuance_flights(id) ON DELETE CASCADE,
    CONSTRAINT issuance_passenger_flights_unique
        UNIQUE (passenger_id, flight_id)
);
