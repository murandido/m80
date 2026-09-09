-- Where a batch of points came from, as a small catalogue instead of free text.
--
-- The hand-kept spreadsheet wrote these by hand, and the same purchase appeared as
-- "LIVELO", "LIVELO 2", "LIVELO 3" within one month - the trailing number being a manual
-- crutch for telling apart three purchases from the same partner. A batch here is
-- identified by its origin and its date, so nothing has to be numbered by hand and a
-- backdated entry never renumbers the ones after it.
--
-- The catalogue is deliberately flat: a transfer partner (Livelo, Esfera), a bank
-- (Inter, C6), a direct purchase from the airline, a club subscription and flight
-- accrual all sit side by side. There is no "kind" column because nothing reads one -
-- what separates a club from a purchase is that its batch costs zero, and that is
-- already on the batch.
CREATE TABLE point_batch_origins (
    id         UUID        NOT NULL DEFAULT gen_random_uuid(),
    name       VARCHAR     NOT NULL,
    -- Retiring an origin hides it from the picker without touching the batches that
    -- already point at it, which is why there is no soft delete here: an archived row
    -- would do the same job while also freeing the name for a second "Livelo". The
    -- delete handler refuses (23503) and points at this flag instead.
    active     BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT point_batch_origins_pkey PRIMARY KEY (id),
    CONSTRAINT point_batch_origins_name_unique UNIQUE (name)
);
