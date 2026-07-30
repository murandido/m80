-- traveos_airline_id is the carrier's id in Traveos' own catalog (GET /airlines/active),
-- which every emission request has to carry. It is nullable because the mapping is not
-- knowable for all 150 carriers and because traveos_supported already gates the flow:
-- a NULL here means "we never mapped this one", which is a different refusal from
-- "we chose not to use Traveos for it".
--
-- brand_color is the carrier's primaryColor from the same catalog, used by the ticket
-- layout. Kept on our side rather than read per-request because the ticket has to render
-- for an issuance whose Traveos data was fetched days ago.
ALTER TABLE airlines
    ADD COLUMN traveos_airline_id INTEGER NULL,
    ADD COLUMN brand_color VARCHAR(7) NULL;
