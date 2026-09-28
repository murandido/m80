-- Reverses 000058. Drops the manual statement-to-issuance links. Nothing derives from them -
-- they are an operator override over a match that is otherwise computed on the fly - so what is
-- lost is the pins, not any settled figure. The auto match keeps working without them.
DROP TABLE IF EXISTS azul_booking_links;
